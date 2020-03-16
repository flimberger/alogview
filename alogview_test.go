/*
 * Copyright (c) 2018-2020 Florian Limberger <flo@purplekraken.com>
 *
 * SPDX-License-Identifier: BSD-2-Clause
 */
package main

import (
	"strings"
	"testing"
)

const testLog = `10-24 22:14:41.150 123 123 D TestTag1: first test message
10-24 22:14:41.150 123 123 D TestTag2: second test message
10-24 22:14:41.150 456 456 D TestTag1: third test message
10-24 22:14:41.150 456 456 D TestTag2: fourth test message
03-16 17:00:42.395  789   789  D Buggy   : about to crash
03-16 17:00:42.396  789   789  F libc    : Fatal signal 6 (SIGABRT), code -6 in tid 789 (com.example.test), pid 789 (com.example.test)
03-16 17:00:42.407  3224  3224 I crash_dump64: obtaining output fd from tombstoned, type: kDebuggerdTombstone
03-16 17:00:42.407  1562  1562 I /system/bin/tombstoned: received crash request for pid 789
03-16 17:00:42.407  3224  3224 I crash_dump64: performing dump of process 789 (target tid = 789)
03-16 17:00:42.407  3224  3224 F DEBUG   : *** *** *** *** *** *** *** *** *** *** *** *** *** *** *** ***
03-16 17:00:42.407  3224  3224 F DEBUG   : Build fingerprint: 'Android/custom/generic_x86_64:8.1.0/OPM1.171019.014/flimbe03120917:eng/test-keys'
03-16 17:00:42.407  3224  3224 F DEBUG   : Revision: '0'
03-16 17:00:42.407  3224  3224 F DEBUG   : pid: 3204, tid: 3204, name: com.example.test  >>> com.example.test <<<
03-16 17:00:42.407  3224  3224 F DEBUG   : signal 6 (SIGABRT), code -6 (SI_TKILL), fault addr --------
03-16 17:00:42.407  3224  3224 F DEBUG   :     rax 0000000000000000  rbx 0000000000000c84  rcx ffffffffffffffff  rdx 0000000000000006
03-16 17:00:42.407  3224  3224 F DEBUG   :     rsi 0000000000000c84  rdi 0000000000000c84
03-16 17:00:42.407  3224  3224 F DEBUG   :     r8  00007f157ce49658  r9  00007f157ce49658  r10 00007f157ce49658  r11 0000000000000246
03-16 17:00:42.407  3224  3224 F DEBUG   :     r12 00007f157356f26b  r13 ea4c7ecc5adf5633  r14 0000000000000c84  r15 00007ffc229d1368
03-16 17:00:42.407  3224  3224 F DEBUG   :     cs  0000000000000033  ss  000000000000002b
03-16 17:00:42.407  3224  3224 F DEBUG   :     rip 00007f160b6766f8  rbp 00007f157ce49658  rsp 00007ffc229d1338  eflags 0000000000000246
03-16 17:00:42.409  3224  3224 F DEBUG   :
03-16 17:00:42.409  3224  3224 F DEBUG   : backtrace:
03-16 17:00:42.409  3224  3224 F DEBUG   :     #00 pc 00000000000276f8  /system/lib64/libc.so (syscall+24)
03-16 17:00:42.409  3224  3224 F DEBUG   :     #01 pc 0000000000027905  /system/lib64/libc.so (abort+101)
03-16 17:00:42.409  3224  3224 F DEBUG   :     #04 pc 0000000000017439  /system/lib64/libcrash.so (crashme+217)
03-16 17:00:42.409  3224  3224 F DEBUG   :     #05 pc 0000000000002882  /system/app/CrashTest/oat/x86_64/CrashTest.odex (offset 0x2000)
03-16 17:00:42.409  3224  3224 F DEBUG   :     #06 pc 000000000000000f  <unknown>`

func runFilters(data string, filters []filter) []*logLine {
	r := strings.NewReader(data)
	acc := []*logLine{}
	rawlines := make(chan *logLine)
	filtered := startFilters(filters, rawlines)
	done := make(chan int)
	go func() {
		for {
			select {
			case line := <-filtered:
				acc = append(acc, line)
			case <-done:
				done <- 1
			}
		}
	}()
	parseLogs(r, rawlines)
	done <- 1
	<-done
	return acc
}

func createPackageFilter() *packageFilter {
	return &packageFilter{
		packages: map[string]bool{"foo": true},
		pids:     map[int]bool{123: true},
	}
}

func TestProcessFiltering(t *testing.T) {
	filters := []filter{createPackageFilter()}
	acc := runFilters(testLog, filters)
	// TODO: this is racy
	if len(acc) != 2 {
		t.Errorf("expected two log entries, got %d instead", len(acc))
	}
	assertProcLine(t, acc[0], 123, "first test message")
	assertProcLine(t, acc[1], 123, "second test message")
}

func assertProcLine(t *testing.T, line *logLine, pid int, msg string) {
	if line.pid != pid {
		t.Errorf("invalid PID: expected: \"%d\" actual: \"%d\"", pid, line.pid)
	}
	if line.message != msg {
		t.Errorf("invalid message: expected: \"%s\" actual: \"%s\"", msg, line.message)
	}
}

func createTagFilter() *tagFilter {
	return &tagFilter{
		tags: map[string]bool{"TestTag1": true},
	}
}

func TestTagFiltering(t *testing.T) {
	filters := []filter{createTagFilter()}
	acc := runFilters(testLog, filters)
	if len(acc) != 2 {
		t.Errorf("expected two log entries, got %d instead", len(acc))
	}
	assertTagLine(t, acc[0], "TestTag1", "first test message")
	assertTagLine(t, acc[1], "TestTag1", "third test message")
}

func assertTagLine(t *testing.T, line *logLine, tag string, msg string) {
	if line.tag != tag {
		t.Errorf("invalid tag: expected: \"%s\" actual: \"%s\"", tag, line.tag)
	}
	if line.message != msg {
		t.Errorf("invalid message: expected: \"%s\" actual: \"%s\"", msg, line.message)
	}
}

func TestProcessAndTagFiltering(t *testing.T) {
	filters := []filter{createPackageFilter(), createTagFilter()}
	acc := runFilters(testLog, filters)
	if len(acc) != 1 {
		t.Errorf("Expected a single log entry, got %d instead", len(acc))
	}
	assertProcLine(t, acc[0], 123, "first test message")
	assertTagLine(t, acc[0], "TestTag1", "first test message")
}

func TestNativeCrashFiltering(t *testing.T) {
	filters := []filter{&packageFilter{
		packages: map[string]bool{"crashing": true},
		pids:     map[int]bool{789: true},
	}}
	acc := runFilters(testLog, filters)
	if len(acc) != 21 {
		t.Errorf("Expected 21 log entries, got %d instead", len(acc))
	}
}
