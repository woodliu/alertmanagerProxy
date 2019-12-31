package msg

import (
    rule "github.com/thanos-io/thanos/pkg/rule"
)


type Message struct {
    Show string  `yaml:"show"`
    RuleGroups rule.RuleGroups `yaml:"rulegroup"`
}
