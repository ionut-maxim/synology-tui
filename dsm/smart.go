package dsm

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// SmartTest is a S.M.A.R.T. self-test type.
type SmartTest string

const (
	SmartQuick    SmartTest = "quick"  // about a minute
	SmartExtended SmartTest = "extend" // hours (a full surface read)
	SmartStop     SmartTest = "stop"   // abort the running test
)

// StartSmartTest starts (or with SmartStop, aborts) a self-test on a disk
// (SYNO.Core.Storage.Disk do_smart_test). Tests read only; data is untouched.
func (c *Client) StartSmartTest(ctx context.Context, device string, t SmartTest) error {
	if err := checkDevice(device); err != nil {
		return err
	}
	switch t {
	case SmartQuick, SmartExtended, SmartStop:
	default:
		return fmt.Errorf("dsm: unknown S.M.A.R.T. test %q", t)
	}
	return c.Call(ctx, "SYNO.Core.Storage.Disk", 1, "do_smart_test",
		url.Values{"device": {device}, "type": {string(t)}}, nil)
}

// SmartStatus is the self-test state of one disk.
type SmartStatus struct {
	Device  string
	Testing bool
	// Progress of the running test ("10%"), as DSM reports it.
	Progress string
	// Last finished test: result ("completed", "aborted", …), type and time.
	LastResult string
	LastType   SmartTest
	LastTime   string
	// DSM's estimates, in minutes.
	QuickMinutes, ExtendedMinutes int
}

// SmartTestStatus reads a disk's self-test state (get_smart_test_log).
func (c *Client) SmartTestStatus(ctx context.Context, device string) (SmartStatus, error) {
	if err := checkDevice(device); err != nil {
		return SmartStatus{}, err
	}
	var out struct {
		LatestTestTime string `json:"latest_test_time"`
		TestInfo       []struct {
			Device     string `json:"device"`
			Testing    bool   `json:"testing"`
			Remain     string `json:"remain"`
			QuickTime  string `json:"quickTime"`
			ExtendTime string `json:"extendTime"`
			LastResult string `json:"latest_test_result"`
			LastType   int    `json:"latest_test_type"`
		} `json:"testInfo"`
	}
	if err := c.Call(ctx, "SYNO.Core.Storage.Disk", 1, "get_smart_test_log", url.Values{"device": {device}}, &out); err != nil {
		return SmartStatus{}, err
	}
	s := SmartStatus{Device: device, LastTime: out.LatestTestTime}
	if len(out.TestInfo) > 0 {
		t := out.TestInfo[0]
		s.Testing, s.Progress, s.LastResult = t.Testing, strings.TrimSpace(t.Remain), t.LastResult
		s.QuickMinutes, _ = strconv.Atoi(strings.TrimSpace(t.QuickTime))
		s.ExtendedMinutes, _ = strconv.Atoi(strings.TrimSpace(t.ExtendTime))
		switch t.LastType {
		case 1:
			s.LastType = SmartQuick
		case 2:
			s.LastType = SmartExtended
		}
	}
	return s, nil
}
