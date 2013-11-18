/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/gomock/gomock"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	pm "github.com/mozilla-services/heka/pipelinemock"
	"github.com/mozilla-services/heka/sandbox"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(FilterSpec)
	//	r.AddSpec(DecoderSpec)

	gs.MainGoTest(r, t)
}

func getTestMessage() *message.Message {
	hostname := "my.host.name"
	field, _ := message.NewField("foo", "bar", "")
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)
	return msg
}

type FilterTestHelper struct {
	MockHelper       *pm.MockPluginHelper
	MockFilterRunner *pm.MockFilterRunner
}

func NewFilterTestHelper(ctrl *gomock.Controller) (fth *FilterTestHelper) {
	fth = new(FilterTestHelper)
	fth.MockHelper = pm.NewMockPluginHelper(ctrl)
	fth.MockFilterRunner = pm.NewMockFilterRunner(ctrl)
	return
}

func FilterSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fth := NewFilterTestHelper(ctrl)
	inChan := make(chan *pipeline.PipelinePack, 1)
	pConfig := pipeline.NewPipelineConfig(nil)

	c.Specify("A SandboxFilter", func() {
		sbFilter := new(SandboxFilter)
		config := sbFilter.ConfigStruct().(*sandbox.SandboxConfig)
		config.ScriptType = "lua"
		config.MemoryLimit = 32000
		config.InstructionLimit = 1000
		config.OutputLimit = 1024

		msg := getTestMessage()
		pack := pipeline.NewPipelinePack(pConfig.InjectRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("Over inject messages from ProcessMessage", func() {
			var timer <-chan time.Time
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("processinject").Times(3)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(2)
			fth.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(2)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../lua/testsupport/processinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			inChan <- pack
			close(inChan)
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Over inject messages from TimerEvent", func() {
			var timer <-chan time.Time
			timer = time.Tick(time.Duration(1) * time.Millisecond)
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("timerinject").Times(12)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(11)
			fth.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(11)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../lua/testsupport/timerinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			go func() {
				time.Sleep(time.Duration(250) * time.Millisecond)
				close(inChan)
			}()
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})
	})

	c.Specify("A SandboxManagerFilter", func() {
		sbmFilter := new(SandboxManagerFilter)
		config := sbmFilter.ConfigStruct().(*SandboxManagerFilterConfig)
		config.MaxFilters = 1

		origBaseDir := pipeline.Globals().BaseDir
		pipeline.Globals().BaseDir = os.TempDir()
		sbxMgrsDir := filepath.Join(pipeline.Globals().BaseDir, "sbxmgrs")
		defer func() {
			pipeline.Globals().BaseDir = origBaseDir
			tmpErr := os.RemoveAll(sbxMgrsDir)
			c.Expect(tmpErr, gs.IsNil)
		}()

		msg := getTestMessage()
		pack := pipeline.NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("Control message in the past", func() {
			sbmFilter.Init(config)
			pack.Message.SetTimestamp(time.Now().UnixNano() - 5e9)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("SandboxManagerFilter")
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("Discarded control message: 5 seconds skew"))
			inChan <- pack
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Control message in the future", func() {
			sbmFilter.Init(config)
			pack.Message.SetTimestamp(time.Now().UnixNano() + 5.9e9)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("SandboxManagerFilter")
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("Discarded control message: -5 seconds skew"))
			inChan <- pack
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Generates the right default working directory", func() {
			sbmFilter.Init(config)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			name := "SandboxManagerFilter"
			fth.MockFilterRunner.EXPECT().Name().Return(name)
			close(inChan)
			sbmFilter.Run(fth.MockFilterRunner, fth.MockHelper)
			c.Expect(sbmFilter.workingDirectory, gs.Equals, sbxMgrsDir)
			_, err := os.Stat(sbxMgrsDir)
			c.Expect(err, gs.IsNil)
		})

	})
}

func DecoderSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A SandboxDecoder", func() {
		decoder := new(SandboxDecoder)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/testsupport/decoder.lua"
		conf.ScriptType = "lua"
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)

		c.Specify("decodes simple messages", func() {
			data := "1376389920 debug id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1376389920000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("id")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "2321")

			value, ok = pack.Message.GetFieldValue("url")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "example.com")

			value, ok = pack.Message.GetFieldValue("item")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "1")
			decoder.Shutdown()
		})

		c.Specify("decodes an invalid messages", func() {
			data := "1376389920 bogus id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pm.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			packs, err := decoder.Decode(pack)
			c.Expect(len(packs), gs.Equals, 0)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
			decoder.Shutdown()
		})
	})
}
