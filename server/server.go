package main

import (
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	rule "github.com/thanos-io/thanos/pkg/rule"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

var (
	p string
	h bool
	ruleFiles string
	cn string
)

const (
    READ_TIMEOUT = time.Millisecond * 10000
	ERR_GROUP_NUM = "wrong rule groups num:There should be only one rule group"
	ERR_NO_RULEFILES = "no rule files found"
	ERR_NO_UPDATEFILE = "can not find file according to the updated rule group"
	ERR_SERVER = "server error"
)
//
func usage() {
	fmt.Fprintf(os.Stderr, `
Note: Only for update existed rule group!
Usage: server [options...]
Options:
`)
	flag.PrintDefaults()
}

func main (){
	flag.BoolVar(&h, "h", false, "help")
	flag.StringVar(&p, "p", "20000", "listen port")
	flag.StringVar(&ruleFiles, "rulefiles", "", "Rule files that should be used by rule manager. Can be in glob format (repeated).")
	flag.StringVar(&cn, "cn", "thanos-ruler", "the container name which need to restart")

	flag.Usage =usage
	flag.Parse()

	//Help info
	if h{
		usage()
		return
	}

	if ruleFiles == ""{
		logrus.Fatalln("Must specify rule files ")
	}

	logrus.Infof("server listen at %s",p)
	l, err := net.Listen("tcp", "0.0.0.0:"+p)
	if err != nil {
		logrus.Fatalln(err)
	}

	defer l.Close()
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			logrus.Fatalln(err)
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {

	defer conn.Close()
	rb := make([]byte, 3000)

	conn.SetDeadline(time.Now().Add(READ_TIMEOUT))

	n,err := conn.Read(rb)
	if nil != err {
		if opErr, ok := err.(*net.OpError); ok {
			if syscallErr, ok := opErr.Err.(*os.SyscallError); ok {
				// may caused be loadbalance healthy check
				if syscallErr.Err == syscall.ECONNRESET {
					return
				}
			}
		}else{
			writeBack(conn, []byte(ERR_SERVER))
		}

		return
	}

	logrus.Infoln("Get info From:",conn.RemoteAddr())
	// Unmarshal updated rules
	var updateRg rule.RuleGroups
	if err := yaml.Unmarshal(rb[:n], &updateRg); err != nil {
		logrus.Errorln(err)
		writeBack(conn, []byte(err.Error()))
		return
	}

	// Because different rule group may exist in different rule files, so limit update only one rule group at once
	if 1 != len(updateRg.Groups){
		writeBack(conn, []byte(ERR_GROUP_NUM))
		return
	}

	// Load all rules from file
	rule_Files, err := filepath.Glob(ruleFiles)
	if nil != err {
		logrus.Fatalln(err)
		return
	}

	if 0 == len(rule_Files){
		logrus.Fatalln(ERR_NO_RULEFILES)
		return
	}

	var updateFile string
	// Find the file to update
	for _, fn := range rule_Files {
		fileBytes, err := ioutil.ReadFile(fn)
		if err != nil {
			logrus.Errorln(err)
			continue
		}

		var oldRg rule.RuleGroups
		if err := yaml.Unmarshal(fileBytes, &oldRg); err != nil {
			logrus.Errorln(err)
			continue
		}

		for k, oldRule := range oldRg.Groups {
			if updateRg.Groups[0].Name == oldRule.Name {
				updateFile = fn
				deepCopy(oldRg.Groups[k],updateRg.Groups[0])
				// write back updated rules to file. When write back to file, some line may splited with "\n",
				// it is allowed to use like that, see https://github.com/go-yaml/yaml/issues/355#issuecomment-379044221
				updatedBytes,err := yaml.Marshal(oldRg)
				// this error may not happen
				if nil != err{
					logrus.Errorln(err)
					writeBack(conn, []byte(ERR_SERVER))
					return
				}

				err = ioutil.WriteFile(updateFile, updatedBytes,777)
				if nil != err{
					logrus.Errorln(err)
					writeBack(conn, []byte(ERR_SERVER))
					return
				}

				goto CONTINUE
			}
		}
	}

	if "" == updateFile{
		writeBack(conn, []byte(ERR_NO_UPDATEFILE))
		return
	}

CONTINUE:
	// restart ruler container
	cmd := exec.Command("docker","restart", cn)
	_, err = cmd.Output()
	if err != nil {
		logrus.Errorln(err.Error())
		writeBack(conn, []byte(ERR_SERVER))
		return
	}

	writeBack(conn,[]byte("config successfully!"))

}

func writeBack(conn net.Conn, bytes []byte){
	_, err := conn.Write(bytes)
	if nil != err {
		logrus.Errorln(err)
		return
	}
}

func deepCopy(dst, src rule.RuleGroup){
	dst.Name = src.Name
	dst.Interval = src.Interval
	for k,_ := range src.Rules{
		dst.Rules[k].Record = src.Rules[k].Record
		dst.Rules[k].Alert = src.Rules[k].Alert
		dst.Rules[k].Expr = src.Rules[k].Expr
		dst.Rules[k].For = src.Rules[k].For
		dst.Rules[k].Labels = src.Rules[k].Labels
		dst.Rules[k].Annotations = src.Rules[k].Annotations
	}

	dst.PartialResponseStrategy = src.PartialResponseStrategy
}