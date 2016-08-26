package proxy

import (
	"errors"
	"strings"
	"sync"

	"code.cloudfoundry.org/lager"

	"github.com/cf-furnace/loggingAgent/retriever"
	"github.com/cf-furnace/pkg/cloudfoundry"
	"github.com/cloudfoundry/dropsonde"
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

func (p *Proxy) Add(pod, container, path string, tail bool) error {
	var source string
	if strings.HasPrefix(container, "application-") {
		source = "APP"
	} else if strings.HasPrefix(container, "staging-") {
		source = "STG"
	} else {
		return errors.New("unsupported-container-name")
	}

	logger := p.logger.WithData(lager.Data{"pod": pod, "path": path})
	randomBits := strings.LastIndexByte(pod, '-')
	if randomBits == -1 {
		logger.Error("pod-name-failure", nil)
		return errors.New("invalid-pod-name")
	}

	pguid, err := cloudfoundry.DecodeProcessGuid(pod[:randomBits])
	if err != nil {
		logger.Error("process-guid-failure", err, lager.Data{"shortened-guid": pod[:randomBits]})
		return errors.New("invalid-process-guid")
	}

	appID := pguid.AppGuid.String()
	r, err := retriever.New(source, appID, path, tail)
	if err != nil {
		logger.Error("new-retriever", err)
		return err
	}

	ino := r.ID()
	if ino == 0 {
		logger.Error("invalid-inode", err)
		return errors.New("invalid-inode")
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

	return nil
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
