package ipnode

import "iptcp-nora-yu/pkg/lnxconfig"

func NewHost(config lnxconfig.IPConfig) (*Node, error) {
	return newNode(config)
}
