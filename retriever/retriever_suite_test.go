package retriever_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"io/ioutil"
	"testing"
)

var tmpDir string

func TestRetriever(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Retriever Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tmpDir, err = ioutil.TempDir("", "retriever")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	os.RemoveAll(tmpDir)
})
