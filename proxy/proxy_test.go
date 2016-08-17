package proxy_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry-incubator/nsync/helpers"
	"github.com/cloudfoundry/dropsonde/emitter/fake"
	"github.com/cloudfoundry/sonde-go/events"
	. "github.com/fraenkel/loggingAgent/proxy"
	"github.com/gogo/protobuf/proto"
	uuid "github.com/nu7hatch/gouuid"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy", func() {
	var (
		logger  *lagertest.TestLogger
		emitter *fake.FakeEventEmitter

		proxy *Proxy
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("")
		emitter = fake.NewFakeEventEmitter("proxy")
		proxy = New(logger, emitter)
	})

	Describe("Add", func() {
		var (
			appGuid *uuid.UUID
			podName string
			logPath string
			tail    bool
		)

		BeforeEach(func() {
			var err error
			appGuid, err = uuid.NewV4()
			Expect(err).NotTo(HaveOccurred())

			pg, err := helpers.NewProcessGuid(appGuid.String() + "-" + appGuid.String())
			Expect(err).NotTo(HaveOccurred())

			podName = pg.ShortenedGuid() + "-rand"
			logPath = "path"
			tail = false
		})

		JustBeforeEach(func() {
			proxy.Add(podName, logPath, tail)
		})

		Context("with an invalid pod name", func() {
			BeforeEach(func() {
				podName = "invalid"
			})

			It("fails with an error", func() {
				Expect(logger.LogMessages()).To(ConsistOf(".proxy.pod-name-failure"))
			})
		})

		Context("with a missing file", func() {
			BeforeEach(func() {
				logPath = "bogus"
			})

			It("fails with an error", func() {
				Expect(logger.LogMessages()).To(ConsistOf(".proxy.invalid-inode"))
			})
		})

		Context("with a valid log", func() {
			var logFile *os.File

			BeforeEach(func() {
				var err error
				logFile, err = ioutil.TempFile(tmpDir, "logfile")
				Expect(err).NotTo(HaveOccurred())
				logPath = logFile.Name()
				logFile.WriteString(`{
					"log": "a stdout message",
					"stream": "out",
					"time": "2009-11-10T23:00:00Z"
				}
				{
					"log": "a stderr message",
					"stream": "err",
					"time": "2009-11-10T23:00:00Z"
				}`)
				logFile.Close()
			})

			It("emits log messages", func() {
				outType := events.LogMessage_OUT
				errType := events.LogMessage_ERR
				appId := appGuid.String()
				Eventually(emitter.GetEvents).Should(ConsistOf(
					&events.LogMessage{
						Message:        []byte("a stdout message"),
						MessageType:    &outType,
						Timestamp:      proto.Int64(1257894000000000000),
						AppId:          proto.String(appId),
						SourceType:     proto.String("APP"),
						SourceInstance: proto.String("??"),
					},
					&events.LogMessage{
						Message:        []byte("a stderr message"),
						MessageType:    &errType,
						Timestamp:      proto.Int64(1257894000000000000),
						AppId:          proto.String(appId),
						SourceType:     proto.String("APP"),
						SourceInstance: proto.String("??"),
					}))
			})

			Context("when the log is deleted", func() {
				It("closes the proxy", func() {
					Eventually(emitter.GetEvents).Should(HaveLen(2))

					os.Remove(logFile.Name())
					Eventually(logger.LogMessages).Should(ContainElement(".proxy.closed"))
				})
			})
		})
	})
})
