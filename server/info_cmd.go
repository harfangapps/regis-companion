package server

import (
	"bytes"
	"expvar"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"bitbucket.org/harfangapps/regis-companion/resp"
)

type infoCmd struct{}

// INFO [section]
func (c infoCmd) Execute(cmdName string, req []string, s *Server) (interface{}, error) {
	if l := len(req); l < 1 || l > 2 {
		return resp.Error(fmt.Sprintf("ERR wrong number of arguments for %v", cmdName)), nil
	}

	var section string
	if len(req) == 2 {
		section = strings.ToLower(req[1])
	}

	var buf bytes.Buffer
	if section == "server" || section == "" {
		execName, _ := os.Executable() // ignore error
		hostName, _ := os.Hostname()   // ignore error
		pid := os.Getpid()
		var port int
		if addr, ok := s.Addr.(*net.TCPAddr); ok {
			port = addr.Port
		}

		fmt.Fprint(&buf, "# Server\r\n")
		fmt.Fprintf(&buf, "regis-companion_version:%s\r\n", Version)
		fmt.Fprintf(&buf, "regis-companion_git_sha1:%s\r\n", GitHash)
		fmt.Fprintf(&buf, "go_version:%s\r\n", runtime.Version())
		fmt.Fprintf(&buf, "go_compiler:%s\r\n", runtime.Compiler)
		fmt.Fprintf(&buf, "os:%s\r\n", runtime.GOOS)
		fmt.Fprintf(&buf, "arch:%s\r\n", runtime.GOARCH)
		fmt.Fprintf(&buf, "process_id:%d\r\n", pid)
		fmt.Fprintf(&buf, "tcp_port:%d\r\n", port)
		fmt.Fprintf(&buf, "executable:%s\r\n", execName)
		fmt.Fprintf(&buf, "hostname:%s\r\n", hostName)
	}

	if section == "memory" || section == "" {
		if buf.Len() > 0 {
			fmt.Fprint(&buf, "\r\n")
		}

		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		fmt.Fprint(&buf, "# Memory\r\n")
		fmt.Fprintf(&buf, "alloc:%d\r\n", stats.Alloc)
		fmt.Fprintf(&buf, "total_alloc:%d\r\n", stats.TotalAlloc)
		fmt.Fprintf(&buf, "sys:%d\r\n", stats.Sys)
		fmt.Fprintf(&buf, "lookups:%d\r\n", stats.Lookups)
		fmt.Fprintf(&buf, "mallocs:%d\r\n", stats.Mallocs)
		fmt.Fprintf(&buf, "frees:%d\r\n", stats.Frees)
		fmt.Fprintf(&buf, "heap_alloc:%d\r\n", stats.HeapAlloc)
		fmt.Fprintf(&buf, "heap_sys:%d\r\n", stats.HeapSys)
		fmt.Fprintf(&buf, "heap_idle:%d\r\n", stats.HeapIdle)
		fmt.Fprintf(&buf, "heap_in_use:%d\r\n", stats.HeapInuse)
		fmt.Fprintf(&buf, "heap_released:%d\r\n", stats.HeapReleased)
		fmt.Fprintf(&buf, "heap_objects:%d\r\n", stats.HeapObjects)
		fmt.Fprintf(&buf, "stack_in_use:%d\r\n", stats.StackInuse)
		fmt.Fprintf(&buf, "stack_sys:%d\r\n", stats.StackSys)
		fmt.Fprintf(&buf, "mspan_in_use:%d\r\n", stats.MSpanInuse)
		fmt.Fprintf(&buf, "mspan_sys:%d\r\n", stats.MSpanSys)
		fmt.Fprintf(&buf, "mcache_in_use:%d\r\n", stats.MCacheInuse)
		fmt.Fprintf(&buf, "mcache_sys:%d\r\n", stats.MCacheSys)
		fmt.Fprintf(&buf, "buck_hash_sys:%d\r\n", stats.BuckHashSys)
		fmt.Fprintf(&buf, "gc_sys:%d\r\n", stats.GCSys)
		fmt.Fprintf(&buf, "other_sys:%d\r\n", stats.OtherSys)
		fmt.Fprintf(&buf, "next_gc:%d\r\n", stats.NextGC)
		fmt.Fprintf(&buf, "last_gc:%d\r\n", stats.LastGC)
		fmt.Fprintf(&buf, "pause_total_ns:%d\r\n", stats.PauseTotalNs)
		fmt.Fprintf(&buf, "num_gc:%d\r\n", stats.NumGC)
		fmt.Fprintf(&buf, "num_forced_gc:%d\r\n", stats.NumForcedGC)
		fmt.Fprintf(&buf, "gc_cpu_fraction:%f\r\n", stats.GCCPUFraction)
	}

	if section == "cpu" || section == "" {
		if buf.Len() > 0 {
			fmt.Fprint(&buf, "\r\n")
		}

		fmt.Fprint(&buf, "# CPU\r\n")
		fmt.Fprintf(&buf, "num_cpu:%d\r\n", runtime.NumCPU())
		fmt.Fprintf(&buf, "num_goroutines:%d\r\n", runtime.NumGoroutine())
		fmt.Fprintf(&buf, "gomaxprocs:%d\r\n", runtime.GOMAXPROCS(0))
	}

	if m := s.Stats; m != nil && (section == "stats" || section == "") {
		if buf.Len() > 0 {
			fmt.Fprint(&buf, "\r\n")
		}

		fmt.Fprint(&buf, "# Stats\r\n")
		m.Do(func(kv expvar.KeyValue) {
			fmt.Fprintf(&buf, "%s:%v\r\n", kv.Key, kv.Value)
		})
	}

	return buf.Bytes(), nil
}
