// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package auditor

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/stretchr/testify/suite"
)

var testpath = "testpath"

type AuditorTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	inputChan chan message.Message
	a         *Auditor
	source    *config.IntegrationConfigLogSource
}

func (suite *AuditorTestSuite) SetupTest() {
	suite.testDir = "tests/"
	os.Remove(suite.testDir)
	os.MkdirAll(suite.testDir, os.ModeDir)
	suite.testPath = fmt.Sprintf("%s/auditor.json", suite.testDir)

	_, err := os.Create(suite.testPath)
	suite.Nil(err)

	suite.inputChan = make(chan message.Message)
	suite.a = New(suite.inputChan)
	suite.a.registryPath = suite.testPath
	suite.source = &config.IntegrationConfigLogSource{Path: testpath}
}

func (suite *AuditorTestSuite) TearDownTest() {
	os.Remove(suite.testDir)
}

func (suite *AuditorTestSuite) TestAuditorUpdatesRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.Equal(0, len(suite.a.registry))
	suite.a.updateRegistry(suite.source.Path, 42)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal(int64(42), suite.a.registry[suite.source.Path].Offset)
	suite.a.updateRegistry(suite.source.Path, 43)
	suite.Equal(int64(43), suite.a.registry[suite.source.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorFlushesAndRecoversRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Path] = &RegistryEntry{
		Path:      suite.source.Path,
		Timestamp: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.Local),
		Offset:    42,
	}
	suite.a.flushRegistry(suite.a.registry, suite.testPath)
	r, err := ioutil.ReadFile(suite.testPath)
	suite.Nil(err)
	suite.Equal("{\"Version\":0,\"Registry\":{\"testpath\":{\"Path\":\"testpath\",\"Timestamp\":\"2006-01-12T01:01:01.000000001Z\",\"Offset\":42}}}", string(r))

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry = suite.a.recoverRegistry(suite.testPath)
	suite.Equal(int64(42), suite.a.registry[suite.source.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorRecoversRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Path] = &RegistryEntry{
		Path:      suite.source.Path,
		Timestamp: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.Local),
		Offset:    42,
	}
	offset, whence := suite.a.GetLastCommitedOffset(suite.source)
	suite.Equal(int64(42), offset)
	suite.Equal(os.SEEK_CUR, whence)

	othersource := &config.IntegrationConfigLogSource{Path: "anotherpath"}
	offset, whence = suite.a.GetLastCommitedOffset(othersource)
	suite.Equal(int64(0), offset)
	suite.Equal(os.SEEK_END, whence)
}

func (suite *AuditorTestSuite) TestAuditorCleansupRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Path] = &RegistryEntry{
		Path:      suite.source.Path,
		Timestamp: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.Local),
		Offset:    42,
	}

	otherpath := "otherpath"
	suite.a.registry[otherpath] = &RegistryEntry{
		Path:      otherpath,
		Timestamp: time.Now(),
		Offset:    43,
	}
	suite.a.flushRegistry(suite.a.registry, suite.testPath)
	suite.Equal(2, len(suite.a.registry))

	suite.a.cleanupRegistry(suite.a.registry)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal(int64(43), suite.a.registry[otherpath].Offset)
}

func (suite *AuditorTestSuite) TestAuditorUnmarshalRegistry() {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": 1,
	            "Path": "path1.log",
	            "Timestamp": "2017-09-05T14:09:29.983303563Z"
	        },
	        "path2.log": {
	            "Offset": 2,
	            "Path": "path2.log",
	            "Timestamp": "2017-09-05T14:09:29.986077799Z"
	        }
	    },
	    "Version": 0
	}`
	r, err := suite.a.unmarshalRegistry([]byte(input))
	suite.Nil(err)
	suite.Equal(r["path1.log"].Offset, int64(1))
	suite.Equal(r["path2.log"].Offset, int64(2))

}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(AuditorTestSuite))
}