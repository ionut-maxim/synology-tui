package dsm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeDSM answers entry.cgi like DSM: form-encoded POSTs, {"success", "data"}.
type fakeDSM struct {
	mu      sync.Mutex
	logins  int
	calls   []string // "api.method"
	expired bool     // next authenticated call fails with 119
	data    map[string]string
}

func (f *fakeDSM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/webapi/entry.cgi" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "parameters must be in the body", http.StatusBadRequest)
		return
	}
	_ = r.ParseForm()
	api, method := r.PostForm.Get("api"), r.PostForm.Get("method")
	f.calls = append(f.calls, api+"."+method)
	if api == "SYNO.API.Auth" && method == "login" {
		if r.PostForm.Get("passwd") != "secret" {
			fmt.Fprint(w, `{"success":false,"error":{"code":400}}`)
			return
		}
		f.logins++
		fmt.Fprintf(w, `{"success":true,"data":{"sid":"sid-%d"}}`, f.logins)
		return
	}
	if r.PostForm.Get("_sid") != fmt.Sprintf("sid-%d", f.logins) || f.expired {
		f.expired = false
		fmt.Fprint(w, `{"success":false,"error":{"code":119}}`)
		return
	}
	if d, ok := f.data[api+"."+method]; ok {
		fmt.Fprintf(w, `{"success":true,"data":%s}`, d)
		return
	}
	fmt.Fprint(w, `{"success":true}`)
}

func setup(t *testing.T, data map[string]string) (*Client, *fakeDSM) {
	t.Helper()
	f := &fakeDSM{data: data}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	c := New(srv.URL, Options{Insecure: true})
	if err := c.Login(context.Background(), "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestLoginErrors(t *testing.T) {
	srv := httptest.NewTLSServer(&fakeDSM{})
	defer srv.Close()
	err := New(srv.URL, Options{Insecure: true}).Login(context.Background(), "admin", "wrong")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || !strings.Contains(err.Error(), "incorrect password") {
		t.Fatalf("err = %v", err)
	}
	if err := New(srv.URL, Options{Insecure: true}).Call(context.Background(), "SYNO.Core.System", 3, "info", nil, nil); err == nil {
		t.Fatal("calls before login must fail")
	}
	// Without Insecure the self-signed test certificate is refused.
	if err := New(srv.URL, Options{}).Login(context.Background(), "admin", "secret"); err == nil {
		t.Fatal("TLS verification must be on by default")
	}
}

func TestExpiredSessionIsRenewed(t *testing.T) {
	c, f := setup(t, map[string]string{"SYNO.Core.System.info": `{"model":"DS420j"}`})
	f.expired = true
	s, err := c.SystemInfo(context.Background())
	if err != nil || s.Model != "DS420j" {
		t.Fatalf("info = %+v, %v", s, err)
	}
	if f.logins != 2 {
		t.Errorf("logins = %d, want a re-login", f.logins)
	}
	if err := (&Error{API: "SYNO.Core.System", Method: "info", Code: 105}); !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("message = %s", err)
	}
}

func TestStorage(t *testing.T) {
	c, _ := setup(t, map[string]string{"SYNO.Storage.CGI.Storage.load_info": `{
		"disks":[{"id":"sdb","device":"/dev/sdb","name":"Drive 2","slot_id":2,"vendor":"Seagate","model":"ST4000VN008",
		          "size_total":"4000787030016","status":"normal","smart_status":"normal","temp":41,"unc":16,"used_by":"reuse_1"}],
		"storagePools":[{"id":"reuse_1","desc":"SHR","device_type":"shr_with_1_disk_protect","status":"attention",
		                 "summary_status":"attention","disks":["sdb"],"size":{"total":"11987517964288","used":"11987517964288"}}],
		"volumes":[{"id":"volume_1","vol_path":"/volume1","fs_type":"ext4","pool_path":"reuse_1","status":"attention",
		            "space_status":{"detail":"fs_almost_full"},"size":{"total":"1000","used":"963"}}]}`})
	s, err := c.Storage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d := s.Disks[0]
	if d.Device != "/dev/sdb" || d.BadSectors != 16 || d.TempC != 41 || d.SizeBytes.String() != "4.0 TB" {
		t.Errorf("disk = %+v (%s)", d, d.SizeBytes)
	}
	if v := s.Volumes[0]; v.SpaceStatus.Detail != "fs_almost_full" || int(v.UsedPercent()) != 96 {
		t.Errorf("volume = %+v", v)
	}
	if p := s.Pools[0]; p.Type != "shr_with_1_disk_protect" || len(p.Disks) != 1 {
		t.Errorf("pool = %+v", p)
	}
}

func TestSmart(t *testing.T) {
	c, f := setup(t, map[string]string{"SYNO.Core.Storage.Disk.get_smart_test_log": `{"latest_test_time":"2026/10/02 23:02:24",
		"testInfo":[{"device":"/dev/sdb","testing":true,"remain":"10%","quickTime":"   1","extendTime":" 622",
		             "latest_test_result":"completed","latest_test_type":1}]}`})
	ctx := context.Background()
	if err := c.StartSmartTest(ctx, "/dev/sdb", SmartExtended); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"sdb", "/dev/sdb; reboot", "/etc/passwd"} {
		if err := c.StartSmartTest(ctx, bad, SmartQuick); err == nil {
			t.Errorf("device %q must be refused", bad)
		}
	}
	if err := c.StartSmartTest(ctx, "/dev/sdb", "format"); err == nil {
		t.Error("unknown test type must be refused")
	}
	s, err := c.SmartTestStatus(ctx, "/dev/sdb")
	if err != nil || !s.Testing || s.Progress != "10%" || s.ExtendedMinutes != 622 || s.LastType != SmartQuick {
		t.Errorf("status = %+v, %v", s, err)
	}
	if n := strings.Count(strings.Join(f.calls, " "), "do_smart_test"); n != 1 {
		t.Errorf("do_smart_test sent %d times; refused inputs must not reach DSM", n)
	}
}

func TestPackages(t *testing.T) {
	c, f := setup(t, map[string]string{"SYNO.Core.Package.list": `{"packages":[
		{"id":"SynologyDrive","name":"Synology Drive Server","version":"4.0.3","additional":{"status":"running"}},
		{"id":"HyperBackup","version":"4.2.2","additional":{"status":"stop"}}]}`})
	ctx := context.Background()
	pkgs, err := c.Packages(ctx)
	if err != nil || len(pkgs) != 2 || pkgs[0].ID != "HyperBackup" || pkgs[0].Name != "HyperBackup" || pkgs[0].Running() || !pkgs[1].Running() {
		t.Fatalf("packages = %+v, %v", pkgs, err)
	}
	f.calls = nil
	if err := c.RestartPackage(ctx, "HyperBackup"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "SYNO.Core.Package.Control.stop,SYNO.Core.Package.Control.start" {
		t.Errorf("restart = %s", got)
	}
	if err := c.StartPackage(ctx, "a b&c"); err == nil {
		t.Error("bad package id must be refused")
	}
}

func TestLogs(t *testing.T) {
	c, _ := setup(t, map[string]string{"SYNO.Core.SyslogClient.Log.list": `{"total":2814,"items":[
		{"time":"2026/10/02 22:23:17","level":"info","logtype":"System","who":"admin","descr":"SNMP service was started."}]}`})
	logs, total, err := c.Logs(context.Background(), 0, 50)
	if err != nil || total != 2814 || len(logs) != 1 || logs[0].Type != "System" {
		t.Fatalf("logs = %+v %d %v", logs, total, err)
	}
}
