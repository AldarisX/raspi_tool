package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/host"
)

var tarTemp float64
var tarPmwPin string
var period int
var increaseStep int
var decreaseStep int
var startTemp float64
var minDutyVal int
var minDuty gpio.Duty

var pmwPin gpio.PinIO
var cpuTemp float64 = 0.0
var tarDuty gpio.Duty
var diffPmwVal gpio.Duty

func init() {
	flag.Float64Var(&tarTemp, "target-temp", 60, "target cpu temp(float64)")
	if tarTemp <= 0 {
		log.Println("target-temp must higher than 0")
		os.Exit(1)
	}
	if tarTemp > 80 {
		log.Println("target-temp must lower than 80")
		os.Exit(1)
	}
	flag.StringVar(&tarPmwPin, "pmw-pin", "18", "target fan pmw pin in BCM pin(string)")
	flag.IntVar(&period, "period", 5, "read cpu temp and adjust fan pmw period in second(int)")
	if period <= 0 {
		log.Println("period must higher than target-temp")
		os.Exit(1)
	}
	flag.IntVar(&increaseStep, "increase-step", 2, "if cpu temp higher than 'target-temp' increase fan pmw duty by this value(int)")
	if tarTemp <= 0 {
		log.Println("increase-step must higher than 0")
		os.Exit(1)
	}
	flag.IntVar(&decreaseStep, "decrease-step", 1, "if cpu temp lower than 'target-temp' decrease fan pmw duty by this value(int)")
	if tarTemp <= 0 {
		log.Println("decrease-step must higher than 0")
		os.Exit(1)
	}
	flag.Float64Var(&startTemp, "start-temp", 40, "if cpu temp higher than this value then start fan(float64)")
	if startTemp >= tarTemp {
		log.Println("start-temp must lower than target-temp")
		os.Exit(1)
	}
	flag.IntVar(&minDutyVal, "min-duty", 10, "fan pmw min value(int)")
	if tarTemp <= 0 {
		log.Println("min-duty must higher than 0")
		os.Exit(1)
	}
	if tarTemp > 99 {
		log.Println("min-duty must lower than 100")
		os.Exit(1)
	}
	minDuty = intToDuty(minDutyVal)
}

func main() {
	flag.Parse()
	host.Init()
	signalChan := make(chan os.Signal, 1) //创建一个信号量的chan，缓存为1，（0,1）意义不大

	signal.Notify(signalChan, syscall.SIGSEGV, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGKILL) //让进程收集信号量。
	// Load all the drivers:
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	pmwPin = gpioreg.ByName(tarPmwPin)

	cpuTempStr := ""
	tarDuty = 0

	cpuTemp, _ = getCPUTemp(cpuTempStr)
	if cpuTemp > tarTemp {
		log.Println("higher than target-temp set duty to 50%")
		tarDuty, _ = gpio.ParseDuty("50%")
	}

	tPMW := time.NewTicker(time.Duration(period) * time.Second)
	go func() {
		for {
			<-tPMW.C

			cpuTemp, _ = getCPUTemp(cpuTempStr)

			log.Println(cpuTemp)

			ctrlPmwFan(cpuTemp)
		}
	}()

	<-signalChan
	exitFunc()
}

func ctrlPmwFan(cpuTemp float64) error {
	if cpuTemp >= startTemp && tarDuty == 0 {
		tarDuty = minDuty
	}
	if cpuTemp < tarTemp {
		diffPmwVal = intToDuty(decreaseStep)
		tarDuty = tarDuty - diffPmwVal
	} else {
		diffPmwVal = intToDuty(increaseStep)
		tarDuty = tarDuty + diffPmwVal
	}
	if tarDuty <= minDuty {
		if cpuTemp < startTemp {
			tarDuty = 0
		} else {
			tarDuty = minDuty
		}
	} else if tarDuty > gpio.DutyMax {
		tarDuty = gpio.DutyMax
	}

	log.Println(tarDuty)
	// 25KHz
	err := pmwPin.PWM(tarDuty, 16*physic.KiloHertz)
	if err != nil {
		return err
	}
	return nil
}

func getCPUTemp(cpuTempStr string) (float64, error) {
	cmd := exec.Command("cat", "/sys/class/thermal/thermal_zone0/temp")
	output, err := cmd.Output()
	if err != nil {
		return 100, err
	}
	cpuTempStr = strings.TrimSpace(string(output))
	cpuTemp, err = strconv.ParseFloat(cpuTempStr, 64)
	if err != nil {
		return 100, err
	}
	cpuTemp = cpuTemp / 1000
	return cpuTemp, nil
}

func exitFunc() {
	log.Println()
	pmwPin.Out(gpio.High)
	log.Println("halt pmwPin")
	pmwPin.Halt()
}

func intToDuty(i int) gpio.Duty {
	return ((gpio.Duty(i) * gpio.DutyMax) + 49) / 100
}

func mapVal(val float32, rawLow float32, rawHigh float32, mapLow float32, mapHigh float32) float32 {
	valInRaw := (val - rawLow) / (rawHigh - rawLow)
	if valInRaw < 0 || valInRaw > 1 {
		return 0
	}
	valMap := (mapHigh-mapLow)*valInRaw + mapLow

	return valMap
}
