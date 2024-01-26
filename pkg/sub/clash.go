package sub

import (
	"gopkg.in/yaml.v3"
)

type ClashSub struct {
	raw *ClashConfig
}

func NewClashSub(rawClashCfgBuf []byte) (*ClashSub, error) {
	var raw ClashConfig
	err := yaml.Unmarshal(rawClashCfgBuf, &raw)
	if err != nil {
		return nil, err
	}
	return &ClashSub{raw: &raw}, nil
}

func (c *ClashSub) ToClashConfigYaml() ([]byte, error) {
	return yaml.Marshal(c.raw)
}
