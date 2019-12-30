package main

import (
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"os"
	"time"
)

var (
	h bool
	t string
	ruleFile string
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Note: Only for update existed rule group!
Usage: server [options...]
Options:
`)
	flag.PrintDefaults()
}

const (
	DIAL_TIMEOUT = time.Millisecond * 10000
	READ_TIMEOUT = time.Millisecond * 10000
)

func main(){
	flag.BoolVar(&h, "h", false, "help")
	flag.StringVar(&t, "t", "", "target")
	flag.StringVar(&ruleFile, "rulefile", "", "rule file")

	flag.Usage =usage
	flag.Parse()

	//Help info
	if h{
		usage()
		return
	}

	if ruleFile == ""{
		logrus.Fatalln("Must specify a rule file")
	}

	if t == ""{
		logrus.Fatalln("Must specify a target")
	}

	conn, err := net.DialTimeout("tcp", t, DIAL_TIMEOUT)
	if err != nil {
		logrus.Fatalln(err)
	}

	defer conn.Close()
	conn.SetDeadline(time.Now().Add(READ_TIMEOUT))

	updateBytes, err := ioutil.ReadFile(ruleFile)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	if 0 == len(updateBytes){
		logrus.Errorln("Nothing to update!")
		return
	}

	_,err = conn.Write(updateBytes)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	rb := make([]byte, 1000)
	n,err := conn.Read(rb)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	logrus.Infoln(string(rb[:n]))
}