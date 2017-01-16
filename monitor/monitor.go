package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/puper/go-lib/server/listener"
)

var (
	addrs   = flag.String("addrs", "", "")
	cmdBin  = flag.String("bin", "", "")
	cmdArgs = flag.String("args", "", "")
	pidFile = flag.String("pid", "", "")
	cmdDir  = flag.String("dir", "", "")
	laddrs  []string

	reloadSignal    chan os.Signal
	stopSignal      chan os.Signal
	cmdSignal       chan *exec.Cmd
	currentCmd      *exec.Cmd
	lastCmdSignalAt = time.Now()
)

func main() {
	flag.Parse()
	var err error
	if *addrs != "" {
		laddrs = strings.Split(*addrs, ",")
	}
	if *cmdBin == "" {
		log.Println("bin empty")
	}

	reloadSignal = make(chan os.Signal, 1)
	stopSignal = make(chan os.Signal, 1)
	cmdSignal = make(chan *exec.Cmd, 1)
	currentCmd, err = startProcess()
	if err != nil {
		log.Println(err)
		return
	}
	signal.Notify(stopSignal,
		os.Kill,
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	signal.Notify(reloadSignal, syscall.SIGHUP)
	writePid(*pidFile)
	handleSignal()
	removePid(*pidFile)

}

func handleSignal() {
	var err error
LOOP:
	for {
		select {
		case <-reloadSignal:
			oldCmd := currentCmd
			for {
				log.Println("reload...")
				currentCmd, err = startProcess()
				if err == nil {
					log.Println("reloaded...")
					stopProcess(oldCmd)
					log.Println("stoped old...")
					break
				}
				log.Println("reload error...")
				time.Sleep(5 * time.Second)
			}
		case <-stopSignal:
			log.Println("stop...")
			stopProcess(currentCmd)
			break LOOP
		case cmd := <-cmdSignal:
			if cmd == currentCmd {
				log.Println("current killed...")
				remain := time.Now().Sub(lastCmdSignalAt)
				if remain < 5*time.Second {
					time.Sleep(5*time.Second - remain)
				}
				lastCmdSignalAt = time.Now()
				for {
					currentCmd, err = startProcess()
					if err == nil {
						break
					}
				}
			}
		}
	}
}

func stopProcess(cmd *exec.Cmd) {
	for {
		cmd.Process.Signal(syscall.SIGTERM)
		fromCmd := <-cmdSignal
		if fromCmd == cmd {
			log.Println("child exited...")
			return
		}
		time.Sleep(time.Second)
	}
}

func startProcess() (*exec.Cmd, error) {
	var err error
	cmd := exec.Command(*cmdBin, *cmdArgs)
	cmd.Dir = *cmdDir
	//cmd.Stderr = os.Stderr
	//cmd.Stdout = os.Stdout
	cmd.ExtraFiles, err = listener.GetFiles(laddrs)
	if err != nil {
		log.Println("files error: " + err.Error())
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		if err != nil {
			log.Println("wait error:" + err.Error())
		}
		cmdSignal <- cmd
	}(cmd)
	return cmd, nil
}

func writePid(path string) {
	if path != "" {
		if err := ioutil.WriteFile(path, []byte(strconv.Itoa(syscall.Getpid())), 0666); err != nil {
			log.Println("write pid file failed: " + err.Error())
		}
	}
}

func removePid(path string) {
	if path != "" {
		pid, err := ioutil.ReadFile(path)
		if err != nil {
			log.Println("read pid file failed: " + err.Error())
		} else {
			if strconv.Itoa(syscall.Getpid()) == string(pid) {
				if err = os.Remove(path); err != nil {
					log.Println("read pid file failed: " + err.Error())
				}
			}
		}
	}
}
