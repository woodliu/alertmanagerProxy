package main

import (
	"alertmanagerProxy/message"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	rule "github.com/thanos-io/thanos/pkg/rule"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"os"
	"time"
)

var (
	h bool
	t string
	show string
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

func main() {

	flag.BoolVar(&h, "h", false, "help")
	flag.StringVar(&t, "t", "", "target")
	flag.StringVar(&show, "show", "", "use 'show all' to show all rule groups; use 'show ${rule_group_name}' to show detail info of a rule group")
	flag.StringVar(&ruleFile, "rulefile", "", "rule file")

	flag.Usage = usage
	flag.Parse()

	//Help info
	if h {
		usage()
		return
	}

	conn, err := net.DialTimeout("tcp", t, DIAL_TIMEOUT)
	if err != nil {
		logrus.Fatalln(err)
	}

	defer conn.Close()
	conn.SetDeadline(time.Now().Add(READ_TIMEOUT))

	var message msg.Message
	var ruleGroups rule.RuleGroups

	message.Show = show

	if t == "" {
		logrus.Fatalln("Must specify a target")
	}

	// do changing
	if show == "" {
		if ruleFile == "" {
			logrus.Fatalln("Must specify a rule file")
		}

		updateBytes, err := ioutil.ReadFile(ruleFile)
		if err != nil {
			logrus.Errorln(err)
			return
		}

		if 0 == len(updateBytes) {
			logrus.Errorln("Nothing to update!")
			return
		}

		if err := yaml.Unmarshal(updateBytes, &ruleGroups); err != nil {
			logrus.Errorln(err)
			return
		}
	}
	message.RuleGroups = ruleGroups
	js, err := yaml.Marshal(message)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	_, err = conn.Write(js)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	rb := make([]byte, 3000)
	n, err := conn.Read(rb)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	fmt.Printf("%s\n", string(rb[:n]))
}