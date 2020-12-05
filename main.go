package main

import (
  "bytes"
  "fmt"
  "log"
  "net/http"
  "strconv"
  "time"
  "encoding/json"
  // "io/ioutil"
  "os"

  linuxproc "github.com/c9s/goprocinfo/linux"
)

//
// cpu usage values are read from the /proc/stat pseudo-file with the help of the goprocinfo package...
// For the calculation two measurements are neaded: 'current' and 'previous'...
// More at: func calcSingleCoreUsage(curr, prev)...
//

type MyCPUStats struct {
  cpu0 float32
  cpu1 float32
  cpu2 float32
  cpu3 float32
}

//
// memory values are read from the /proc/meminfo pseudo-file with the help of the goprocinfo package...
// how are they calculated? Like 'htop' command, see question:
//   - http://stackoverflow.com/questions/41224738/how-to-calculate-memory-usage-from-proc-meminfo-like-htop/
//

type MyMemoInfo struct {
  TotalMachine       uint64
  TotalUsed          uint64
  Buffers            uint64
  Cached             uint64
  NonCacheNonBuffers uint64
}

//
// memory values are read from the /proc/meminfo pseudo-file with the help of the goprocinfo package...
// how are they calculated? Like 'htop' command, see question:
//   - http://stackoverflow.com/questions/41224738/how-to-calculate-memory-usage-from-proc-meminfo-like-htop/
//

type MonitoringData  struct {
  Cpu0          float32   `json:"cpu0"`
  Cpu1          float32   `json:"cpu1"`
  Cpu2          float32   `json:"cpu2"`
  Cpu3          float32   `json:"cpu3"`
  MemoryUsage   uint64    `json:"memoryUsage"`
}

func main() {
  time_interval := 1 // this number represents seconds
  push_to_influx := false
  print_std_out := false
  // targetFileName := "current_status_tmp"

  influxUrl := "http://10.143.0.218:8086"
  cpuDBname := "pi_cpu"
  memoDBname := "pi_memo"

  currCPUStats := readCPUStats()
  prevCPUStats := readCPUStats()
  info := readMemoInfo()

  fmt.Println("Monitoring started")

  for {
    time.Sleep(time.Second * time.Duration(time_interval))

    //
    //  CPU stuff below...
    //

    currCPUStats = readCPUStats()
    coreStats := calcMyCPUStats(currCPUStats, prevCPUStats)

    if print_std_out {
      fmt.Printf("CPU stats: %+v", coreStats)
    }

    if push_to_influx {
      url := influxUrl + "/write?db=" + cpuDBname
      body := []byte("cpu0,coreID=0 value=" + strconv.FormatFloat(float64(coreStats.cpu0), 'f', -1, 32) + "\n" +
        "cpu1,coreID=1 value=" + strconv.FormatFloat(float64(coreStats.cpu0), 'f', -1, 32) + "\n" +
        "cpu2,coreID=2 value=" + strconv.FormatFloat(float64(coreStats.cpu0), 'f', -1, 32) + "\n" +
        "cpu3,coreID=3 value=" + strconv.FormatFloat(float64(coreStats.cpu0), 'f', -1, 32))
      // fmt.Printf("%s", body)
      req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
      hc := http.Client{}
      _, err = hc.Do(req)
      if err != nil {
        log.Fatal("could not send POST")
      }
    }
    prevCPUStats = currCPUStats

    //
    //  Memory stuff below...
    //

    info = readMemoInfo()

    var mmInfo MyMemoInfo
    mmInfo.TotalMachine = info.MemTotal
    mmInfo.TotalUsed = info.MemTotal - info.MemFree
    mmInfo.Buffers = info.Buffers
    mmInfo.Cached = info.Cached + info.SReclaimable - info.Shmem
    mmInfo.NonCacheNonBuffers = mmInfo.TotalUsed - (mmInfo.Buffers + mmInfo.Cached)

    if print_std_out {
      fmt.Printf(" | Memory info: %+v\n", mmInfo)
    }

    if push_to_influx {
      url := influxUrl + "/write?db=" + memoDBname
      body := []byte("TotalMachine,memTag=TotalMachine value=" + strconv.Itoa(int(mmInfo.TotalMachine)) + "\n" +
        "TotalUsed,memTag=TotalUsed value=" + strconv.Itoa(int(mmInfo.TotalUsed)) + "\n" +
        "Buffers,memTag=Buffers value=" + strconv.Itoa(int(mmInfo.Buffers)) + "\n" +
        "Cached,memTag=Cached value=" + strconv.Itoa(int(mmInfo.Cached)) + "\n" +
        "NonCacheNonBuffers,memTag=NonCacheNonBuffers value=" + strconv.Itoa(int(mmInfo.NonCacheNonBuffers)))

      req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
      hc := http.Client{}
      _, err = hc.Do(req)
      if err != nil {
        log.Fatal("could not send POST")
      }
    }

    monitoringData := MonitoringData {
      Cpu0: coreStats.cpu0,
      Cpu1: coreStats.cpu1,
      Cpu2: coreStats.cpu2,
      Cpu3: coreStats.cpu3,
      MemoryUsage: mmInfo.TotalUsed}

    jsonMonitoringData, _ := json.Marshal(monitoringData)
    fmt.Println(string(jsonMonitoringData))

    fileToOutput, err := os.Create("current_status.json")
    check(err)
    defer fileToOutput.Close()

    fileToOutput.WriteString(string(jsonMonitoringData))
    fileToOutput.Sync()
  }
}


// -------------- END OF MAIN ------------------


func readCPUStats() *linuxproc.Stat {
  stat, err := linuxproc.ReadStat("/proc/stat")
  if err != nil {
    log.Fatal("stat read fail")
  }

  return stat
}

func calcMyCPUStats(curr, prev *linuxproc.Stat) *MyCPUStats {
  var stats MyCPUStats

  stats.cpu0 = calcSingleCoreUsage(curr.CPUStats[0], prev.CPUStats[0])
  stats.cpu1 = calcSingleCoreUsage(curr.CPUStats[1], prev.CPUStats[1])
  stats.cpu2 = calcSingleCoreUsage(curr.CPUStats[2], prev.CPUStats[2])
  stats.cpu3 = calcSingleCoreUsage(curr.CPUStats[3], prev.CPUStats[3])


  return &stats
}

/*
 *    source: http://stackoverflow.com/questions/23367857/accurate-calculation-of-cpu-usage-given-in-percentage-in-linux
 *
 *    PrevIdle = previdle + previowait
 *    Idle = idle + iowait
 *
 *    PrevNonIdle = prevuser + prevnice + prevsystem + previrq + prevsoftirq + prevsteal
 *    NonIdle = user + nice + system + irq + softirq + steal
 *
 *    PrevTotal = PrevIdle + PrevNonIdle
 *    Total = Idle + NonIdle
 *
 *    # differentiate: actual value minus the previous one
 *    totald = Total - PrevTotal
 *    idled = Idle - PrevIdle
 *
 *    CPU_Percentage = (totald - idled)/totald
 */

func calcSingleCoreUsage(curr, prev linuxproc.CPUStat) float32 {
  PrevIdle := prev.Idle + prev.IOWait
  Idle := curr.Idle + curr.IOWait

  PrevNonIdle := prev.User + prev.Nice + prev.System + prev.IRQ + prev.SoftIRQ + prev.Steal
  NonIdle := curr.User + curr.Nice + curr.System + curr.IRQ + curr.SoftIRQ + curr.Steal

  PrevTotal := PrevIdle + PrevNonIdle
  Total := Idle + NonIdle

  //  differentiate: actual value minus the previous one
  totald := Total - PrevTotal
  idled := Idle - PrevIdle

  CPU_Percentage := (float32(totald) - float32(idled)) / float32(totald)

  return CPU_Percentage
}

//
//  Memory
//
//

func readMemoInfo() *linuxproc.MemInfo {
  info, err := linuxproc.ReadMemInfo("/proc/meminfo")
  if err != nil {
    log.Fatal("info read fail")
  }

  return info
}

func check(e error) {
    if e != nil {
        panic(e)
    }
}