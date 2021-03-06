package main

import (
	"alertmanagerProxy/message"
	"flag"
	"fmt"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/sirupsen/logrus"
	rule "github.com/thanos-io/thanos/pkg/rule"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		logrus.WithFields(logrus.Fields{"function": "main",}).Fatalln("Must specify rule files ")
	}

	logrus.Infof("server listen at %s",p)
	l, err := net.Listen("tcp", "0.0.0.0:"+p)
	if err != nil {
		logrus.WithFields(logrus.Fields{"function": "main",}).Fatalln(err.Error())
	}

	defer l.Close()
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			logrus.WithFields(logrus.Fields{"function": "main",}).Fatalln(err.Error())
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

	logrus.WithFields(logrus.Fields{"function": "main",}).Infoln("Get info From:",conn.RemoteAddr())

	var message msg.Message
	err = yaml.Unmarshal(rb[:n],&message)
	if nil != err {
		logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
		writeBack(conn, []byte(ERR_SERVER))
		return
	}

	// Load all rules from file
	rule_Files, err := filepath.Glob(ruleFiles)
	if nil != err {
		logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
		return
	}

	if 0 == len(rule_Files){
		logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(ERR_NO_RULEFILES)
		return
	}

	if message.Show == "all" {
		showAllGroups(rule_Files, conn)
		return
	}else if message.Show != "" {
		showGroupByName(rule_Files, message.Show, conn)
		return
	}

	// Because different rule group may exist in different rule files, so limit update only one rule group at once
	if 1 != len(message.RuleGroups.Groups){
		writeBack(conn, []byte(ERR_GROUP_NUM))
		return
	}

	var updateFile string
	// Find the file to update
	for _, fn := range rule_Files {
		fileBytes, err := ioutil.ReadFile(fn)
		if err != nil {
			logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
			continue
		}

		var oldRg rule.RuleGroups
		if err := yaml.Unmarshal(fileBytes, &oldRg); err != nil {
			logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
			continue
		}

		for k, oldRule := range oldRg.Groups {
			if message.RuleGroups.Groups[0].Name == oldRule.Name {
				updateFile = fn
				deepCopy(&oldRg.Groups[k],&message.RuleGroups.Groups[0])
				// write back updated rules to file. When write back to file, some line may splited with "\n",
				// it is allowed to use like that, see https://github.com/go-yaml/yaml/issues/355#issuecomment-379044221
				updatedBytes,err := yaml.Marshal(oldRg)
				// this error may not happen
				if nil != err{
					logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
					writeBack(conn, []byte(ERR_SERVER))
					return
				}

				err = ioutil.WriteFile(updateFile, updatedBytes,777)
				if nil != err{
					logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
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
		logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
		writeBack(conn, []byte(ERR_SERVER))
		return
	}

	writeBack(conn,[]byte("config successfully!"))

}

func writeBack(conn net.Conn, bytes []byte){
	_, err := conn.Write(bytes)
	if nil != err {
		logrus.WithFields(logrus.Fields{"function": "main",}).Errorln(err.Error())
		return
	}
}

func deepCopy(dst, src *rule.RuleGroup){
	dst.Rules = []rulefmt.Rule{}

	dst.Name = src.Name
	dst.Interval = src.Interval
	for _,v := range src.Rules {
		dst.Rules = append(dst.Rules,v)
	}

/*
	var newSrcRules []rulefmt.Rule
	var deleteDstKeys []int

	dst.Name = src.Name
	dst.Interval = src.Interval
	for k1,v1 := range src.Rules{
		for k2,v2 := range dst.Rules{
			if v1.Alert == v2.Alert{
				dst.Rules[k2].Record = src.Rules[k1].Record
				dst.Rules[k2].Expr = src.Rules[k1].Expr
				dst.Rules[k2].For = src.Rules[k1].For
				dst.Rules[k2].Labels = src.Rules[k1].Labels
				dst.Rules[k2].Annotations = src.Rules[k1].Annotations
				goto CONTINUE1
			}
		}

		newSrcRules = append(newSrcRules,v1)
CONTINUE1:
	}

	// delete rules
	for k1,v1 := range dst.Rules{
		for _,v2 := range src.Rules{
			if v1.Alert == v2.Alert{
				goto CONTINUE2
			}
		}

		deleteDstKeys = append(deleteDstKeys,k1)
CONTINUE2:
	}

	for i:=len(deleteDstKeys)-1; i>=0; i-- {
		dst.Rules = append(dst.Rules[:deleteDstKeys[i]], dst.Rules[deleteDstKeys[i]+1:]...)
	}

	// add new rules
	dst.Rules = append(dst.Rules,newSrcRules...)
	dst.PartialResponseStrategy = src.PartialResponseStrategy
*/
}

func showAllGroups(ruleFiles []string, conn net.Conn) {
	var groupNames = []string{"<----Show Rule Groups---->"}
	for _, fn := range ruleFiles {
		fileBytes, err := ioutil.ReadFile(fn)
		if err != nil {
			logrus.WithFields(logrus.Fields{"function": "showAllGroups",}).Errorln(err.Error())
			writeBack(conn, []byte(err.Error()))
			return
		}

		var rg rule.RuleGroups
		if err := yaml.Unmarshal(fileBytes, &rg); err != nil {
			logrus.WithFields(logrus.Fields{"function": "showAllGroups",}).Errorln(err.Error())
			writeBack(conn, []byte(err.Error()))
			return
		}

		for _, group := range rg.Groups {
			groupNames = append(groupNames, group.Name)
		}
	}

	newNewroupNames := strings.Join(groupNames,"\r\n")
	conn.Write([]byte(newNewroupNames))
}

func showGroupByName(ruleFiles []string, groupName string, conn net.Conn){
	var groupBytes []byte
	var rulegroups rule.RuleGroups

	for _, fn := range ruleFiles {
		fileBytes, err := ioutil.ReadFile(fn)
		if err != nil {
			logrus.WithFields(logrus.Fields{"function": "showGroupByName",}).Errorln(err.Error())
			writeBack(conn, []byte(err.Error()))
			return
		}

		var rg rule.RuleGroups
		if err := yaml.Unmarshal(fileBytes, &rg); err != nil {
			logrus.WithFields(logrus.Fields{"function": "showGroupByName",}).Errorln(err.Error())
			writeBack(conn, []byte(err.Error()))
			return
		}

		for _, group := range rg.Groups {
			if  groupName == group.Name {
				rulegroups.Groups = append(rulegroups.Groups,group)
				groupBytes,err = yaml.Marshal(rulegroups)
				if err != nil {
					logrus.WithFields(logrus.Fields{"function": "showGroupByName",}).Errorln(err.Error())
					writeBack(conn, []byte(ERR_SERVER))
					return
				}
			}
		}
	}

	if 0 == len(groupBytes){
		res := fmt.Sprintf("search failed for: [%s]\n",groupName)
		conn.Write([]byte(res))
		return
	}
	conn.Write(groupBytes)
}
