package main

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

func podFQDN(pod *v1.Pod) string {
	return fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, cfg.RSSvc, cfg.NS)
}

func podFQDNAndPort(pod *v1.Pod) string {
	return podFQDN(pod) + ":" + strconv.FormatUint(uint64(cfg.MDBPort), 10)
}

func podOrd(pod *v1.Pod) (uint8, error) {
	var hyphenIdx int
	for hyphenIdx = len(pod.Name) - 1; hyphenIdx >= 0 && pod.Name[hyphenIdx] != '-'; hyphenIdx-- {
	}
	if hyphenIdx == -1 {
		return 0, errors.New("pod name does not match <statefulset>-<ordinal> format")
	}
	ord, err := strconv.ParseUint(pod.Name[hyphenIdx+1:], 10, 8)
	return uint8(ord), errors.WithStack(err)
}
