package proxy

import (
	"encoding/base32"
	"errors"
	"strings"
	"sync"

	"code.cloudfoundry.org/lager"

	"github.com/cf-furnace/loggingAgent/retriever"
	"github.com/cloudfoundry-incubator/nsync/helpers"
	"github.com/cloudfoundry/dropsonde"
	uuid "github.com/nu7hatch/gouuid"
)

type Proxy struct {
	logger       lager.Logger
	eventEmitter dropsonde.EventEmitter

	mu          sync.Mutex
	inodesToApp map[uint64]*retriever.LogReader
}

func New(logger lager.Logger, eventEmitter dropsonde.EventEmitter) *Proxy {
	return &Proxy{
		logger:       logger.Session("proxy"),
		eventEmitter: eventEmitter,
		inodesToApp:  map[uint64]*retriever.LogReader{},
	}
}

func (p *Proxy) Add(pod string, path string, tail bool) {
	logger := p.logger.WithData(lager.Data{"pod": pod, "path": path})
	randomBits := strings.LastIndexByte(pod, '-')
	if randomBits == -1 {
		logger.Error("pod-name-failure", nil)
		return
	}

	pguid, err := decodeProcessGuid(pod[:randomBits])
	if err != nil {
		logger.Error("process-guid-failure", err, lager.Data{"shortened-guid": pod[:randomBits]})
		return
	}

	appID := pguid.AppGuid.String()
	r, err := retriever.New(appID, path, tail)
	if err != nil {
		logger.Error("new-retriever", err)
		return
	}

	ino := r.ID()
	if ino == 0 {
		logger.Error("invalid-inode", err)
		return
	}

	p.mu.Lock()
	if _, exists := p.inodesToApp[ino]; !exists {
		p.inodesToApp[ino] = r
	} else {
		r = nil
	}
	p.mu.Unlock()

	logger.Info("read-logs")
	go func() {
		p.copyEvents(logger, appID, r)
		p.mu.Lock()
		delete(p.inodesToApp, ino)
		p.mu.Unlock()
	}()
}

func (p *Proxy) copyEvents(logger lager.Logger, appID string, logReader *retriever.LogReader) {
	logger = logger.WithData(lager.Data{"appID": appID})
	for {
		select {
		case msg, ok := <-logReader.Msg:
			if !ok {
				logger.Info("closed")
				return
			}

			err := p.eventEmitter.Emit(msg)
			if err != nil {
				logger.Error("failed-to-emit-event", err)
			}
		case err := <-logReader.Err:
			logger.Error("failed-to-copy-events", err)
			return
		}
	}
}

func decodeProcessGuid(shortenedGuid string) (helpers.ProcessGuid, error) {
	splited := strings.Split(strings.ToUpper(shortenedGuid), "-")
	if len(splited) != 2 {
		return helpers.ProcessGuid{}, errors.New("invalid shortened process guid")
	}
	// add padding
	appGuid := addPadding(splited[0])
	appVersion := addPadding(splited[1])

	// decode it
	longAppGuid, err := base32.StdEncoding.DecodeString(appGuid[:])
	if err != nil {
		return helpers.ProcessGuid{}, errors.New("Unable to decode appGuid - invalid shortened process guid")
	}
	longAppVersion, err := base32.StdEncoding.DecodeString(appVersion[:])
	if err != nil {
		return helpers.ProcessGuid{}, errors.New("Unable to decode appVersion - invalid shortened process guid")
	}

	appGuidUUID, err := uuid.Parse(longAppGuid)
	appVersionUUID, err := uuid.Parse(longAppVersion)

	if err != nil {
		return helpers.ProcessGuid{}, errors.New("Unable to parse appGuid - invalid shortened process guid")
	}

	if err != nil {
		return helpers.ProcessGuid{}, errors.New("Unable to parse appVersion - invalid shortened process guid")
	}

	return helpers.NewProcessGuid(appGuidUUID.String() + "-" + appVersionUUID.String())
}

func addPadding(s string) string {
	return s + strings.Repeat("=", 8-len(s)%8)
}
