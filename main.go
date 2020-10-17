/*

rtop - the remote system monitoring utility

Copyright (c) 2015 RapidLoop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	//"os/signal"
	//"syscall"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

        "github.com/prometheus/client_golang/prometheus"
        "net/http"
        "github.com/prometheus/client_golang/prometheus/promhttp"
)

const VERSION = "1.0"
var currentUser *user.User

//----------------------------------------------------------------------------
var (
  tempDesc0 = prometheus.NewDesc(
    "a_cpu_idle", "CPU Usage (Idle)", nil, nil,
  )
  tempDesc1 = prometheus.NewDesc(
    "a_cpu_user", "CPU Usage (User)", nil, nil,
  )
  tempDesc2 = prometheus.NewDesc(
    "a_mem_free", "Memory (Free)", nil, nil,
  )
  tempDesc3 = prometheus.NewDesc(
    "a_mem_used", "Memory (Used)", nil, nil,
  )
)

type ClusterManagerCollector struct {
}

func (cc ClusterManagerCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

func (cc ClusterManagerCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	allTargetStats := getstats()

	for _, oneTargetStats := range allTargetStats {
		mystats := oneTargetStats.theStats
		//log.Printf("%d %d %d", mystats.CPU.User, mystats.CPU.Idle, mystats.MemFree)

    ch <- prometheus.MustNewConstMetric(
      tempDesc0,
      prometheus.GaugeValue,
      float64(mystats.CPU.Idle),
    )
    ch <- prometheus.MustNewConstMetric(
      tempDesc1,
      prometheus.GaugeValue,
      float64(mystats.CPU.User),
    )
    ch <- prometheus.MustNewConstMetric(
      tempDesc2,
      prometheus.GaugeValue,
      float64(mystats.MemFree),
    )

    used := mystats.MemTotal - mystats.MemFree - mystats.MemBuffers - mystats.MemCached
    ch <- prometheus.MustNewConstMetric(
      tempDesc3,
      prometheus.GaugeValue,
      float64(used),
    )
  }
	duration := time.Since(start)
	log.Print(duration)
}

type IpNodeStats struct {
	Ip       string
	daStats  Stats
}

func makeRequest(ch chan<- IpNodeStats, ip string, wg *sync.WaitGroup) {
	defer wg.Done()
	getstats()
}

type TargetStats struct {
	theStats  *Stats
	theTarget *Target
}

type Target struct {
	User    string
	Ip      string
	Port    int
	lastCpu cpuRaw
}

var AllTargets []Target

func ParseFile() {
	file, err := os.Open("./rtop-clients")
	if err != nil {
		log.Print(err)
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		//log.Printf(scanner.Text())
		words := strings.Fields(scanner.Text())
		//log.Printf(words[0], words[1], words[2])

		if i, err := strconv.Atoi(words[2]); err == nil {
			t := Target{words[0], words[1], i, cpuRaw{}}
			AllTargets = append(AllTargets, t)
		}
	}

	file.Close()
}

func main() {
	cc := ClusterManagerCollector{}
	prometheus.MustRegister(cc)
	ParseFile()

        http.Handle("/metrics", promhttp.Handler())
        log.Printf("Beginning to serve on port :8090")
        log.Fatal(http.ListenAndServe(":8090", nil))
}

func getstats() ([]TargetStats) {

	// get current user
	var err error
	currentUser, err = user.Current()
	if err != nil {
		log.Print(err)
	}

	idrsap := filepath.Join(currentUser.HomeDir, ".ssh", "id_rsa")
	var key string
	if _, err := os.Stat(idrsap); err == nil {
		key = idrsap
	}
	if key == "" {
		panic("no key found")
	}

	var fullStats []TargetStats

	for counter, onetarget := range AllTargets {
		host := onetarget.Ip
		port := onetarget.Port
		username := onetarget.User
		log.Printf("%s %d %s %s", host, port, username, key)

		addr := fmt.Sprintf("%s:%d", host, port)
		client := sshConnect(username, addr, key)

		//shouldn't need this channel
		//sig := make(chan os.Signal, 1)
		//signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		stats := Stats{}
		//log.Printf("before %d", onetarget.lastCpu.Total)
		AllTargets[counter].lastCpu = getAllStats(client, &stats, onetarget.lastCpu)
		//log.Printf("after %d", AllTargets[counter].lastCpu.Total)
		log.Printf("%.2f %.2f %d", stats.CPU.User, stats.CPU.Idle, stats.MemFree)

		ts := TargetStats{&stats, &onetarget}
		fullStats = append(fullStats, ts)
	}

	return fullStats
}


func showStats(output io.Writer, client *ssh.Client) {
	stats := Stats{}
	//getAllStats(client, &stats)
	clearConsole()
	used := stats.MemTotal - stats.MemFree - stats.MemBuffers - stats.MemCached
	fmt.Fprintf(output,
		`%s%s%s%s up %s%s%s

Load:
    %s%s %s %s%s

CPU:
    %s%.2f%s%% user, %s%.2f%s%% sys, %s%.2f%s%% nice, %s%.2f%s%% idle, %s%.2f%s%% iowait, %s%.2f%s%% hardirq, %s%.2f%s%% softirq, %s%.2f%s%% guest

Processes:
    %s%s%s running of %s%s%s total

Memory:
    free    = %s%s%s
    used    = %s%s%s
    buffers = %s%s%s
    cached  = %s%s%s
    swap    = %s%s%s free of %s%s%s

`,
		escClear,
		escBrightWhite, stats.Hostname, escReset,
		escBrightWhite, fmtUptime(&stats), escReset,
		escBrightWhite, stats.Load1, stats.Load5, stats.Load10, escReset,
		escBrightWhite, stats.CPU.User, escReset,
		escBrightWhite, stats.CPU.System, escReset,
		escBrightWhite, stats.CPU.Nice, escReset,
		escBrightWhite, stats.CPU.Idle, escReset,
		escBrightWhite, stats.CPU.Iowait, escReset,
		escBrightWhite, stats.CPU.Irq, escReset,
		escBrightWhite, stats.CPU.SoftIrq, escReset,
		escBrightWhite, stats.CPU.Guest, escReset,
		escBrightWhite, stats.RunningProcs, escReset,
		escBrightWhite, stats.TotalProcs, escReset,
		escBrightWhite, fmtBytes(stats.MemFree), escReset,
		escBrightWhite, fmtBytes(used), escReset,
		escBrightWhite, fmtBytes(stats.MemBuffers), escReset,
		escBrightWhite, fmtBytes(stats.MemCached), escReset,
		escBrightWhite, fmtBytes(stats.SwapFree), escReset,
		escBrightWhite, fmtBytes(stats.SwapTotal), escReset,
	)
	if len(stats.FSInfos) > 0 {
		fmt.Println("Filesystems:")
		for _, fs := range stats.FSInfos {
			fmt.Fprintf(output, "    %s%8s%s: %s%s%s free of %s%s%s\n",
				escBrightWhite, fs.MountPoint, escReset,
				escBrightWhite, fmtBytes(fs.Free), escReset,
				escBrightWhite, fmtBytes(fs.Used+fs.Free), escReset,
			)
		}
		fmt.Println()
	}
	if len(stats.NetIntf) > 0 {
		fmt.Println("Network Interfaces:")
		keys := make([]string, 0, len(stats.NetIntf))
		for intf := range stats.NetIntf {
			keys = append(keys, intf)
		}
		sort.Strings(keys)
		for _, intf := range keys {
			info := stats.NetIntf[intf]
			fmt.Fprintf(output, "    %s%s%s - %s%s%s",
				escBrightWhite, intf, escReset,
				escBrightWhite, info.IPv4, escReset,
			)
			if len(info.IPv6) > 0 {
				fmt.Fprintf(output, ", %s%s%s\n",
					escBrightWhite, info.IPv6, escReset,
				)
			} else {
				fmt.Fprintf(output, "\n")
			}
			fmt.Fprintf(output, "      rx = %s%s%s, tx = %s%s%s\n",
				escBrightWhite, fmtBytes(info.Rx), escReset,
				escBrightWhite, fmtBytes(info.Tx), escReset,
			)
			fmt.Println()
		}
		fmt.Println()
	}
}

const (
	escClear       = "\033[H\033[2J"
	escRed         = "\033[31m"
	escReset       = "\033[0m"
	escBrightWhite = "\033[37;1m"
)
