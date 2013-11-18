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
#   Bryan Zubrod (bzubrod@gmail.com)
#   Victor Ng (vng@mozilla.com)
#
#***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// ManagedCmd extends exec.Cmd to support killing of a subprocess if
// a timeout has been exceeded.  A timeout duration value of 0
// indicates that no timeout is enforced.
type ManagedCmd struct {
	exec.Cmd

	Path string
	Args []string
	// Env specifies the environment of the process.
	// If Env is nil, Run uses the current process's environment.
	Env []string

	// Dir specifies the working directory of the command.
	// If Dir is the empty string, Run runs the command in the
	// calling process's current directory.
	Dir string

	done     chan error
	Stopchan chan bool

	// Note that the timeout duration is only used when Wait() is called.
	// If you put this command on a run interval where the interval time is
	// very close to the timeout interval, it is possible that the
	// timeout may only occur *after* the command has been restarted.
	timeout_duration time.Duration

	Stdout_r *io.PipeReader
	Stderr_r *io.PipeReader

	Stdout_chan chan string
	Stderr_chan chan string
}

func NewManagedCmd(path string, args []string, timeout time.Duration) (mc *ManagedCmd) {
	mc = &ManagedCmd{Path: path, Args: args, timeout_duration: timeout}
	mc.done = make(chan error)
	mc.Stopchan = make(chan bool, 1)
	mc.Cmd = *exec.Command(mc.Path, mc.Args...)
	mc.Cmd.Env = mc.Env
	mc.Cmd.Dir = mc.Dir

	mc.Stdout_chan = make(chan string)
	mc.Stderr_chan = make(chan string)
	return mc
}

func (mc *ManagedCmd) Start(redirectToChannels bool) (err error) {
	if redirectToChannels {
		var stdout_w *io.PipeWriter
		var stderr_w *io.PipeWriter

		mc.Stdout_r, stdout_w = io.Pipe()
		mc.Stderr_r, stderr_w = io.Pipe()

		mc.Cmd.Stdout = stdout_w
		mc.Cmd.Stderr = stderr_w

		// Process stdout
		go func() {
			var err error
			var buffer []byte
			var bytes_read int

			buffer = make([]byte, 500)
			for {
				bytes_read, err = mc.Stdout_r.Read(buffer)
				if bytes_read > 0 {
					mc.Stdout_chan <- string(buffer[:bytes_read])
				}
				if err != nil {
					close(mc.Stdout_chan)
					return
				}
			}
		}()

		// Process stderr
		go func() {
			var err error
			var buffer []byte
			var bytes_read int

			buffer = make([]byte, 1000)
			for {
				bytes_read, err = mc.Stderr_r.Read(buffer)
				if bytes_read > 0 {
					mc.Stderr_chan <- string(buffer[:bytes_read])
				}
				if err != nil {
					close(mc.Stderr_chan)
					return
				}
			}
		}()
	}

	return mc.Cmd.Start()
}

// We overload the Wait() method to enable subprocess termination if a
// timeout has been exceeded.
func (mc *ManagedCmd) Wait() (err error) {
	go func() {
		mc.done <- mc.Cmd.Wait()
	}()

	if mc.timeout_duration != 0 {
		select {
		case <-mc.Stopchan:
			err = fmt.Errorf("CommandChain was stopped with error: [%s]", mc.kill())
		case <-time.After(mc.timeout_duration):
			err = fmt.Errorf("CommandChain timedout with error: [%s]", mc.kill())
		case err = <-mc.done:
		}
	} else {
		select {
		case <-mc.Stopchan:
			err = fmt.Errorf("CommandChain was stopped with error: [%s]", mc.kill())
		case err = <-mc.done:
		}
	}

	var writer *io.PipeWriter
	var ok bool

	writer, ok = mc.Stdout.(*io.PipeWriter)
	if ok {
		writer.Close()
	}
	writer, ok = mc.Stderr.(*io.PipeWriter)
	if ok {
		writer.Close()
	}

	return err
}

// Kill the current process. This will always return an error code.
func (mc *ManagedCmd) kill() (err error) {
	if err := mc.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill subprocess: %s", err.Error())
	}
	// killing process will make Wait() return
	<-mc.done
	return fmt.Errorf("subprocess was killed: [%s %s]", mc.Path, strings.Join(mc.Args, " "))
}

// This resets a command so that we can run the command again.
// Usually so that a chain can be restarted.
func (mc *ManagedCmd) clone() (clone *ManagedCmd) {
	clone = NewManagedCmd(mc.Path, mc.Args, mc.timeout_duration)
	return clone
}

func (mc *ManagedCmd) StdoutChan() (stream chan string) {
	return mc.Stdout_chan
}

func (mc *ManagedCmd) StderrChan() (stream chan string) {
	return mc.Stderr_chan
}

// A CommandChain lets you execute an ordered set of subprocesses
// and pipe stdout to stdin for each stage.
type CommandChain struct {
	Cmds []*ManagedCmd

	// The timeout duration is the maximum time that each stage of the
	// pipeline should run for before the Wait() returns a
	// timeout error.
	timeout_duration time.Duration

	done     chan error
	Stopchan chan bool
}

func NewCommandChain(timeout time.Duration) (cc *CommandChain) {
	cc = &CommandChain{timeout_duration: timeout}
	cc.done = make(chan error)
	cc.Stopchan = make(chan bool, 1)
	return cc
}

// Add a single command to our command chain, piping stdout to stdin
// for each stage.
func (cc *CommandChain) AddStep(Path string, Args ...string) (cmd *ManagedCmd) {
	cmd = NewManagedCmd(Path, Args, cc.timeout_duration)

	cc.Cmds = append(cc.Cmds, cmd)
	if len(cc.Cmds) > 1 {
		r, w := io.Pipe()
		cc.Cmds[len(cc.Cmds)-2].Stdout = w
		cc.Cmds[len(cc.Cmds)-1].Stdin = r
	}
	return cmd
}

func (cc *CommandChain) StdoutChan() (stream chan string, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].Stdout_chan, nil
}

func (cc *CommandChain) StderrChan() (stream chan string, err error) {
	if len(cc.Cmds) == 0 {
		return nil, fmt.Errorf("No commands are in this chain")
	}
	return cc.Cmds[len(cc.Cmds)-1].Stderr_chan, nil
}

func (cc *CommandChain) Start() (err error) {
	/* This is a bit subtle.  You want to spin up all the commands in
	   order by calling Start().  */

	for idx, cmd := range cc.Cmds {
		if idx == (len(cc.Cmds) - 1) {
			err = cmd.Start(true)
		} else {
			err = cmd.Start(false)
		}

		if err != nil {
			return fmt.Errorf("Command [%s %s] triggered an error: [%s]",
				cmd.Path,
				strings.Join(cmd.Args, " "),
				err.Error())
		}
	}
	return nil
}

func (cc *CommandChain) Wait() (err error) {
	/* You need to Wait and close the stdout for each
	   stage in order, except that you do *not* want to close the last
	   output pipe as we need to use that to get the final results.  */
	go func() {
		var subcmd_err error
		for i, cmd := range cc.Cmds {
			subcmd_err = cmd.Wait()
			if subcmd_err != nil {
				cc.done <- subcmd_err
				return
			}
			if i < (len(cc.Cmds) - 1) {
				subcmd_err = cmd.Stdout.(*io.PipeWriter).Close()
				if subcmd_err != nil {
					cc.done <- subcmd_err
					return
				}
			}
		}
		cc.done <- nil
	}()

	select {
	case err = <-cc.done:
		return err
	case <-cc.Stopchan:
		for i := len(cc.Cmds) - 1; i >= 0; i-- {
			cmd := cc.Cmds[i]
			cmd.Stopchan <- true
		}
		return fmt.Errorf("Chain stopped")
	}
	return nil
}

// This resets a command so that we can run the command again.
// Usually so that a chain can be restarted.
func (cc *CommandChain) clone() (clone *CommandChain) {
	clone = NewCommandChain(cc.timeout_duration)
	for _, cmd := range cc.Cmds {
		clone.AddStep(cmd.Path, cmd.Args...)
	}
	return clone
}

type StringChannelReader struct {
	input  chan string
	buffer string
}

func (scr *StringChannelReader) Read(p []byte) (n int, err error) {
	// This call blocks until we get some data
	select {
	case inbound := <-scr.input:
		if len(inbound) > 0 {
			scr.buffer += inbound
		} else {
			// The channel is closed
			err = io.EOF
			break
		}
	}

	if len(scr.buffer) <= len(p) {
		n = copy(p, []byte(scr.buffer))
		scr.buffer = ""
	} else {
		n = copy(p, []byte(scr.buffer[:len(p)]))
		scr.buffer = scr.buffer[len(p):]
	}
	return n, err
}
