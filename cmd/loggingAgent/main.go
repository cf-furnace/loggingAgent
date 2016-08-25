package main

import (
	"flag"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"code.cloudfoundry.org/cflager"

	"github.com/cf-furnace/loggingAgent/proxy"
	"github.com/cf-furnace/loggingAgent/watcher"
	"github.com/cloudfoundry/dropsonde"
)

const (
	dropsondeOrigin = "loggingAgent"
)

var logsDir = flag.String(
	"logsDir",
	"/var/log/containers",
	"directory containing the kubernetes' container logs",
)
var dropsondePort = flag.Int(
	"dropsondePort",
	3457,
	"port the local metron agent is listening on",
)

func main() {
	cflager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cflager.New("logging-agent")

	destination := "127.0.0.1:" + strconv.Itoa(*dropsondePort)
	err := dropsonde.Initialize(destination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed-to-initialize-dropsonde", err)
		os.Exit(1)
	}

	watchEvents, err := watcher.Watch(logger, *logsDir)
	if err != nil {
		logger.Error("failed-to-initialize-watcher", err)
		os.Exit(1)
	}

	logProxy := proxy.New(logger, dropsonde.AutowiredEmitter())

	osSignals := make(chan os.Signal, 5)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)

DONE:
	for {
		select {
		case event := <-watchEvents:
			if strings.HasPrefix(event.Container, "application-") {
				logProxy.Add(event.Pod, event.Path, event.Info != nil)
			}
		case <-osSignals:
			signal.Stop(osSignals)
			break DONE
		}
	}

	logger.Info("exited")
}
