package main

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

const loopSleep = 5 * time.Second

func main() {
	for {
		fmt.Printf("%+v\n", cfg)
		err := mainBody()
		if err != nil {
			fmt.Printf("ERROR: %+v\n", err)
		}
		time.Sleep(loopSleep)
	}
}

func mainBody() error {
	ps, err := k8sGetPods()
	if err != nil {
		return err
	}

	// Get running mongo pods.
	var runningPs []*v1.Pod
	for i := range ps {
		p := &ps[i]
		if p.Status.Phase == v1.PodRunning && p.Status.PodIP != "" {
			runningPs = append(runningPs, p)
		}
	}
	if len(runningPs) == 0 {
		return errors.New("No running mongo pods. Waiting.")
	}

	// Check replica set status.
	status, err := mongoReplSetGetStatus()
	if err != nil {
		return err
	}
	fmt.Printf("%+v\n", status)

	return nil
}
