package dsm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// Storage is SYNO.Storage.CGI.Storage load_info: what Storage Manager shows.
type Storage struct {
	Disks   []Disk   `json:"disks"`
	Pools   []Pool   `json:"storagePools"`
	Volumes []Volume `json:"volumes"`
}

type Disk struct {
	ID          string `json:"id"`     // "sdb"
	Device      string `json:"device"` // "/dev/sdb": what S.M.A.R.T. calls take
	Name        string `json:"name"`   // "Drive 2"
	Slot        int    `json:"slot_id"`
	Vendor      string `json:"vendor"`
	Model       string `json:"model"`
	Firmware    string `json:"firm"`
	Type        string `json:"diskType"` // SATA, SAS, M.2 …
	SSD         bool   `json:"isSsd"`
	SizeBytes   Bytes  `json:"size_total"`
	Status      string `json:"status"`       // normal, warning, crashed …
	SmartStatus string `json:"smart_status"` // normal, abnormal …
	TempC       int    `json:"temp"`
	// Uncorrectable (bad) sectors, from S.M.A.R.T.
	BadSectors int    `json:"unc"`
	UsedBy     string `json:"used_by"` // pool id
}

type Pool struct {
	ID            string   `json:"id"`
	Description   string   `json:"desc"`        // "SHR"
	Type          string   `json:"device_type"` // "shr_with_1_disk_protect"
	Status        string   `json:"status"`
	SummaryStatus string   `json:"summary_status"`
	Disks         []string `json:"disks"`
	// SpaceStatus.Detail explains an "attention" status, e.g. "fs_almost_full".
	SpaceStatus struct {
		Detail string `json:"detail"`
	} `json:"space_status"`
	Size struct {
		Total Bytes `json:"total"`
		Used  Bytes `json:"used"`
	} `json:"size"`
}

type Volume struct {
	ID            string `json:"id"`       // "volume_1"
	Path          string `json:"vol_path"` // "/volume1"
	Description   string `json:"desc"`
	FS            string `json:"fs_type"`
	Pool          string `json:"pool_path"`
	Status        string `json:"status"`
	SummaryStatus string `json:"summary_status"`
	// SpaceStatus.Detail explains an "attention" status, e.g. "fs_almost_full".
	SpaceStatus struct {
		Detail string `json:"detail"`
	} `json:"space_status"`
	Size struct {
		Total Bytes `json:"total"`
		Used  Bytes `json:"used"`
	} `json:"size"`
}

// UsedPercent of the volume.
func (v Volume) UsedPercent() float64 {
	if v.Size.Total == 0 {
		return 0
	}
	return float64(v.Size.Used) / float64(v.Size.Total) * 100
}

// Bytes is a size DSM sends as a JSON string ("4000787030016").
type Bytes uint64

func (b *Bytes) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		*b = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("size %s: %w", data, err)
	}
	*b = Bytes(n)
	return nil
}

// String is a human size in decimal units, as DSM shows disks (4.0 TB).
func (b Bytes) String() string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", uint64(b))
	}
	div, exp := uint64(unit), 0
	for n := uint64(b) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

// Storage returns disks, pools and volumes.
func (c *Client) Storage(ctx context.Context) (Storage, error) {
	var s Storage
	err := c.Call(ctx, "SYNO.Storage.CGI.Storage", 1, "load_info", nil, &s)
	return s, err
}

var deviceRe = regexp.MustCompile(`^/dev/[a-z0-9]+$`)

func checkDevice(device string) error {
	if !deviceRe.MatchString(device) {
		return fmt.Errorf("dsm: bad disk device %q (want e.g. /dev/sda)", device)
	}
	return nil
}
