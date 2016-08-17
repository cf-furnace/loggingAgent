package watcher

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Pod       string
	Namespace string
	Container string
	Path      string

	Info os.FileInfo
}

var kubeTagRegexp = regexp.MustCompile(`([^_]+)_([^_]+)_(.+)`)

func Watch(logger lager.Logger, logDir string) (<-chan *Event, error) {
	logger = logger.Session("Watcher", lager.Data{"logDir": logDir})
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	err = watcher.Add(logDir)
	if err != nil {
		watcher.Close()
		return nil, err
	}

	newFiles := make(chan *Event, 10)

	go func() {
		currentLogs(logger, logDir, newFiles)

		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create {
					if evt := toEvent(event.Name); evt != nil {
						newFiles <- evt
					}
				}
			case err := <-watcher.Errors:
				logger.Error("watcher failed", err)
				return
			}
		}
	}()

	return newFiles, nil
}

func currentLogs(logger lager.Logger, logDir string, newFiles chan<- *Event) {
	d, err := os.Open(logDir)
	if err != nil {
		logger.Error("currentLogs-open", err)
		return
	}

	files, err := d.Readdir(0)
	if err != nil {
		logger.Error("currentLogs-readdir", err)
		return
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if evt := toEvent(filepath.Join(logDir, f.Name())); evt != nil {
			evt.Info = f
			newFiles <- evt
		}
	}
}

func toEvent(pth string) *Event {
	if !strings.HasSuffix(pth, ".log") {
		return nil
	}

	name := path.Base(pth)
	name = name[:strings.LastIndex(name, ".")]
	tags := kubeTagRegexp.FindStringSubmatch(name)
	if len(tags) != 4 {
		return nil
	}

	return &Event{
		Pod:       tags[1],
		Namespace: tags[2],
		Container: tags[3],
		Path:      pth,
	}
}
