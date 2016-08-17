package retriever_test

import (
	"io/ioutil"
	"os"

	. "github.com/cf-furnace/loggingAgent/retriever"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("JsonLogFile", func() {
	var appID string
	var jsonLog *os.File
	var tail bool

	var reader *LogReader

	BeforeEach(func() {
		appID = "appID"
		tail = false

		var err error
		jsonLog, err = ioutil.TempFile(tmpDir, "jsonlog")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		var err error
		reader, err = New(appID, jsonLog.Name(), tail)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(jsonLog.Name())
	})

	Context("when the file fails to open", func() {
		BeforeEach(func() {
			jsonLog.Close()
			os.Remove(jsonLog.Name())
		})

		It("sends an error", func() {
			var err error
			Eventually(reader.Err).Should(Receive(&err))
			Expect(err).To(BeAssignableToTypeOf(&os.PathError{}))
		})
	})

	Context("with valid json", func() {
		BeforeEach(func() {
			jsonLog.WriteString(`{
					"log": "a stdout message",
					"stream": "out",
					"time": "2009-11-10T23:00:00Z"
				}
				{
					"log": "a stderr message",
					"stream": "err",
					"time": "2009-11-10T23:00:00Z"
				}`)
			jsonLog.Close()
		})

		It("reads until the end of the file", func() {
			var e *events.LogMessage
			Eventually(reader.Msg).Should(Receive(&e))
			msgType := events.LogMessage_OUT
			Expect(e).To(Equal(&events.LogMessage{
				Message:        []byte("a stdout message"),
				MessageType:    &msgType,
				Timestamp:      proto.Int64(1257894000000000000),
				AppId:          proto.String("appID"),
				SourceType:     proto.String("APP"),
				SourceInstance: proto.String("??"),
			}))
			Eventually(reader.Msg).Should(Receive(&e))
			msgType = events.LogMessage_ERR
			Expect(e).To(Equal(&events.LogMessage{
				Message:        []byte("a stderr message"),
				MessageType:    &msgType,
				Timestamp:      proto.Int64(1257894000000000000),
				AppId:          proto.String("appID"),
				SourceType:     proto.String("APP"),
				SourceInstance: proto.String("??"),
			}))
		})
	})

	Context("with a partial json line", func() {
		BeforeEach(func() {
			jsonLog.WriteString(`{
					"log": "a stdout message",
					"stream": "out",
					"time": "2009-11-10T23:00:00Z"`)
		})

		It("waits until the line is valid json", func() {
			Consistently(reader.Msg).ShouldNot(Receive())
			jsonLog.WriteString(`}`)
			jsonLog.Close()
			Eventually(reader.Msg).Should(Receive())
		})
	})

	Context("when the file is rolled", func() {
		BeforeEach(func() {
			jsonLog.WriteString(`{
					"log": "a stdout message",
					"stream": "out",
					"time": "2009-11-10T23:00:00Z"
				}
				{
					"log": "a stderr message",
					"stream": "err",
					"time": "2009-11-10T23:00:00Z"`)
		})

		JustBeforeEach(func() {
			Eventually(reader.Msg).Should(Receive())
			err := os.Rename(jsonLog.Name(), jsonLog.Name()+".1")
			Expect(err).NotTo(HaveOccurred())
			jsonLog.WriteString(`}`)
			jsonLog.Close()
		})

		It("BROKEN: loses the last line", func() {
			Eventually(reader.Msg).Should(BeClosed())
		})
	})
})
