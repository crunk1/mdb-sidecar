package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	v1 "k8s.io/api/core/v1"
)

const loopSleep = 15 * time.Second

func main() {
	for {
		err := mainBody()
		if err != nil {
			fmt.Printf("ERROR: %+v\n", err)
		}
		time.Sleep(loopSleep)
	}
}

func mainBody() error {
	pods, err := k8sGetPods()
	if err != nil {
		return err
	}

	// Get replica set status. Additional handling if not in a replica set.
	status, err := mongoReplSetGetStatus(nil)
	if err != nil {
		cmdErr, ok := errors.Cause(err).(mongo.CommandError)
		if !ok {
			return err
		}
		if cmdErr.Code == 93 {
			// Invalid replica set. Don't know how to handle yet.
			return err
		} else if cmdErr.Code == 94 {
			// Not in replica set.
			return mainNotInReplSet(pods)
		} else {
			return err
		}
	}

	// Check if this pod is PRIMARY. If not PRIMARY, nothing to do.
	membersStatuses, ok := status["members"].(primitive.A)
	if !ok {
		return errors.New("can't parse members from replSet status")
	}

	isPrimary := false
	for _, memberStatusI := range membersStatuses {
		memberStatus, ok := memberStatusI.(map[string]interface{})
		if !ok {
			return errors.New("can't parse member from replSet status members")
		}
		if memberStatus["name"] == cfg.podFQDNAndPort && memberStatus["stateStr"] == "PRIMARY" {
			isPrimary = true
			break
		}
	}
	if !isPrimary {
		return nil
	}

	return mainPrimaryWork(pods)
}

// mainNotInReplSet:
//  1. looks for an existing replica set among the other pods; if found:
//    a. returns a wait error, this pod will eventually get picked up by the replica set.
//  2. if this pod is the "first" pod of the statefulset, i.e. "<statefulset>-0":
//    a. creates the replica set.
//  3. else:
//    a. waits for the "first" pod to create the replica set and pick this pod up.
func mainNotInReplSet(pods []v1.Pod) error {
	// Check other pods. Return a wait error if we find one.
	for _, p := range pods {
		if p.Name == cfg.PodName || p.Status.Phase != v1.PodRunning || p.Status.PodIP == "" {
			continue // skip self and non-running pods
		}
		in, err := mongoIsInReplSet(&p)
		if in {
			fmt.Println("existing replica set found, waiting.")
			return nil
		} else if err != nil {
			return err
		}
	}

	// If this is "first" pod, create replica set.
	firstName := cfg.RSSvc + "-0"
	if firstName == cfg.PodName {
		err := mongoInitReplSet(pods)
		if err != nil {
			return err
		}
	}
	// Else, return an error to wait for "first" pod to pick us up.
	return errors.Errorf("replica set needs created and this is not pod %q, waiting", firstName)
}

// mainPrimaryWork is the work for the PRIMARY:
// - add/remove members from replica set as k8s service changes pod members
func mainPrimaryWork(pods []v1.Pod) error {
	err := mainPrimaryWorkSyncReplSetMembers(pods)
	if err != nil {
		return err
	}

	err = mainPrimaryWorkSyncPrimaryK8sService(pods)
	if err != nil {
		return err
	}

	return nil
}

// mainPrimaryWorkSyncReplSetMembers syncs mongo replica set members with the
// k8s service's pods.
func mainPrimaryWorkSyncReplSetMembers(pods []v1.Pod) error {
	// PRIMARY work: sync replica set members with k8s service pods.
	rsConfig, err := mongoReplSetGetConfig(nil)
	if err != nil {
		return err
	}
	rsConfig, ok := rsConfig["config"].(map[string]interface{})
	if !ok {
		return errors.New("can't parse replica set config")
	}
	members, ok := rsConfig["members"].(primitive.A)
	if !ok {
		return errors.New("can't parse replica set config members")
	}
	var newMembers []map[string]interface{}
	for _, pod := range pods {
		ord, err := podOrd(&pod)
		if err != nil {
			return err
		}
		newMembers = append(newMembers, map[string]interface{}{
			"_id":  ord,
			"host": podFQDNAndPort(&pod),
		})
	}
	sort.Slice(newMembers, func(i int, j int) bool { return newMembers[i]["_id"].(uint8) < newMembers[j]["_id"].(uint8) })
	// Check if replica set members are equal to the k8s service pods. If equal, nothing to do.
	if len(members) == len(newMembers) {
		equal := true
		for i, member := range members {
			m, ok := member.(map[string]interface{})
			if !ok {
				return errors.New("can't parse replica set config member")
			}
			if m["_id"] != newMembers[i]["_id"] || m["host"] != newMembers[i]["host"] {
				equal = false
				break
			}
		}
		if equal {
			return nil
		}
	}
	rsConfig["members"] = newMembers
	return mongoReplSetReconfig(nil, rsConfig)
}

func mainPrimaryWorkSyncPrimaryK8sService(pods []v1.Pod) error {
	key := cfg.RSSvc + "-primary"
	for _, pod := range pods {
		var err error
		if pod.Name == cfg.PodName {
			err = k8sAddPodLabel(&pod, key, "true")
		} else {
			err = k8sRemovePodLabel(&pod, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
