package watcher_test

import (
	"io/ioutil"
	"os"
	"path"

	"code.cloudfoundry.org/lager/lagertest"

	"github.com/cf-furnace/loggingAgent/watcher"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Watcher", func() {
	var tmpDir string

	var createdChan <-chan *watcher.Event
	var existingName string
	var existingFile *os.File

	BeforeEach(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "watcher")
		Expect(err).NotTo(HaveOccurred())

		existingName = "existing_namespace_cnr.log"
		existingFile, err = os.OpenFile(path.Join(tmpDir, existingName), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		logger := lagertest.NewTestLogger("watcher")

		var err error
		createdChan, err = watcher.Watch(logger, tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	Context("when a log file exists", func() {
		It("fires an event", func() {
			var event *watcher.Event
			Eventually(createdChan).Should(Receive(&event))
			fi := event.Info
			event.Info = nil
			Expect(event).To(Equal(&watcher.Event{Pod: "existing", Namespace: "namespace", Container: "cnr", Path: existingFile.Name()}))
			Expect(fi).NotTo(BeNil())
		})

		Context("when the file is rotated", func() {
			var existingInfo os.FileInfo

			JustBeforeEach(func() {
				var event *watcher.Event
				Eventually(createdChan).Should(Receive(&event))
				existingInfo = event.Info

				oldFile := path.Join(tmpDir, existingName)
				err := os.Rename(oldFile, oldFile+".1")
				Expect(err).NotTo(HaveOccurred())
				existingFile, err = os.OpenFile(oldFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fires an event for the new file", func() {
				var event *watcher.Event
				Eventually(createdChan).Should(Receive(&event))
				Expect(event).To(Equal(&watcher.Event{Pod: "existing", Namespace: "namespace", Container: "cnr", Path: existingFile.Name()}))
				f, err := os.Open(event.Path)
				Expect(err).NotTo(HaveOccurred())
				s, err := f.Stat()
				Expect(err).NotTo(HaveOccurred())
				Expect(s.Sys).ToNot(Equal(existingInfo.Sys))
			})
		})
	})

	Context("when a log file is created", func() {
		var name string
		var newFile *os.File

		BeforeEach(func() {
			name = "pod_namespace_cnr.log"
		})

		JustBeforeEach(func() {
			Eventually(createdChan).Should(Receive())

			var err error
			newFile, err = os.OpenFile(path.Join(tmpDir, name), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fires an event", func() {
			var event *watcher.Event
			Eventually(createdChan).Should(Receive(&event))
			Expect(event).To(Equal(&watcher.Event{Pod: "pod", Namespace: "namespace", Container: "cnr", Path: newFile.Name()}))
		})
	})

	Context("when a generic file is created", func() {
		JustBeforeEach(func() {
			Eventually(createdChan).Should(Receive())

			_, err := os.OpenFile(path.Join(tmpDir, "some.log"), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
			Expect(err).NotTo(HaveOccurred())

		})

		It("does not fire an event", func() {
			Consistently(createdChan).ShouldNot(Receive())
		})
	})
})
