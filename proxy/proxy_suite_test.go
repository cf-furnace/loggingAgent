package proxy_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

var tmpDir string

func TestProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tmpDir, err = ioutil.TempDir("", "retriever")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	os.RemoveAll(tmpDir)
})
