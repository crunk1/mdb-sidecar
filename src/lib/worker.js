let mongo = require('./mongo');
let k8s = require('./k8s');
let config = require('./config');
let ip = require('ip');
let async = require('async');
let moment = require('moment');
let dns = require('dns');
let os = require('os');

let loopSleepSeconds = config.loopSleepSeconds;
let unhealthySeconds = config.unhealthySeconds;

let hostIp = false;
let hostIpAndPort = false;

let init = function(done) {
  //Borrowed from here: http://stackoverflow.com/questions/3653065/get-local-ip-address-in-node-js
  let hostName = os.hostname();
  dns.lookup(hostName, function (err, addr) {
    if (err) {
      return done(err);
    }

    hostIp = addr;
    hostIpAndPort = hostIp + ':' + config.mongoPort;

    done();
  });
};

let workloop = function workloop() {
  if (!hostIp || !hostIpAndPort) {
    throw new Error('Must initialize with the host machine\'s addr');
  }

  //Do in series so if k8s.getMongoPods fails, it doesn't open a db connection
  async.series([
    k8s.getMongoPods,
    mongo.getDb
  ], function(err, results) {
    let db = null;
    if (Array.isArray(results) && results.length === 2) {
      db = results[1];
    }

    if (err) {
      return finish(err, db);
    }

    let pods = results[0];

    //Lets remove any pods that aren't running or haven't been assigned an IP address yet
    for (let i = pods.length - 1; i >= 0; i--) {
      let pod = pods[i];
      if (pod.status.phase !== 'Running' || !pod.status.podIP) {
        pods.splice(i, 1);
      }
    }

    if (!pods.length) {
      return finish('No pods are currently running, probably just give them some time.');
    }

    //Lets try and get the rs status for this mongo instance
    //If it works with no errors, they are in the rs
    //If we get a specific error, it means they aren't in the rs
    mongo.replSetGetStatus(db, function(err, status) {
      if (err) {
        if (err.code && err.code == 94) {
          notInReplicaSet(db, pods, function(err) {
            finish(err, db);
          });
        }
        else if (err.code && err.code == 93) {
          invalidReplicaSet(db, pods, status, function(err) {
            finish(err, db);
          });
        }
        else {
          finish(err, db);
        }
        return;
      }

      inReplicaSet(db, pods, status, function(err) {
        finish(err, db);
      });
    });
  });
};

let finish = function(err, db) {
  if (err) {
    console.error('Error in workloop', err);
  }

  if (db && db.close) {
    db.close();
  }

  setTimeout(workloop, loopSleepSeconds * 1000);
};

let inReplicaSet = function(db, pods, status, done) {
  //If we're already in a rs and we ARE the primary, do the work of the primary instance (i.e. adding others)
  //If we're already in a rs and we ARE NOT the primary, just continue, nothing to do
  //If we're already in a rs and NO ONE is a primary, elect someone to do the work for a primary
  let members = status.members;

  let primaryExists = false;
  for (let i in members) {
    let member = members[i];

    if (member.state === 1) {
      if (member.self) {
        return primaryWork(db, pods, members, false, done);
      }

      primaryExists = true;
      break;
    }
  }

  if (!primaryExists && podElection(pods)) {
    console.log('Pod has been elected as a secondary to do primary work');
    return primaryWork(db, pods, members, true, done);
  }

  done();
};

let primaryWork = function(db, pods, members, shouldForce, done) {
  //Loop over all the pods we have and see if any of them aren't in the current rs members array
  //If they aren't in there, add them
  let addrToAdd = addrToAddLoop(pods, members);
  let addrToRemove = addrToRemoveLoop(members);

  if (addrToAdd.length || addrToRemove.length) {
    console.log('Addresses to add:    ', addrToAdd);
    console.log('Addresses to remove: ', addrToRemove);

    mongo.addNewReplSetMembers(db, addrToAdd, addrToRemove, shouldForce, done);
    return;
  }

  done();
};

let notInReplicaSet = function(db, pods, done) {
  let createTestRequest = function(pod) {
    return function(completed) {
      mongo.isInReplSet(pod.status.podIP, completed);
    };
  };

  //If we're not in a rs and others ARE in the rs, just continue, another path will ensure we will get added
  //If we're not in a rs and no one else is in a rs, elect one to kick things off
  let testRequests = [];
  for (let i in pods) {
    let pod = pods[i];

    if (pod.status.phase === 'Running') {
      testRequests.push(createTestRequest(pod));
    }
  }

  async.parallel(testRequests, function(err, results) {
    if (err) {
      return done(err);
    }

    for (let i in results) {
      if (results[i]) {
        return done(); //There's one in a rs, nothing to do
      }
    }

    if (podElection(pods)) {
      console.log('Pod has been elected for replica set initialization');
      let primary = pods[0]; // After the sort election, the 0-th pod should be the primary.
      let primaryStableNetworkAddressAndPort = getPodStableNetworkAddressAndPort(primary);
      // Prefer the stable network ID over the pod IP, if present.
      let primaryAddressAndPort = primaryStableNetworkAddressAndPort || hostIpAndPort;
      mongo.initReplSet(db, primaryAddressAndPort, done);
      return;
    }

    done();
  });
};

let invalidReplicaSet = function(db, pods, status, done) {
  // The replica set config has become invalid, probably due to catastrophic errors like all nodes going down
  // this will force re-initialize the replica set on this node. There is a small chance for data loss here
  // because it is forcing a reconfigure, but chances are recovering from the invalid state is more important
  let members = [];
  if (status && status.members) {
    members = status.members;
  }

  console.log("Invalid replica set");
  if (!podElection(pods)) {
    console.log("Didn't win the pod election, doing nothing");
    return done();
  }

  console.log("Won the pod election, forcing re-initialization");
  let addrToAdd = addrToAddLoop(pods, members);
  let addrToRemove = addrToRemoveLoop(members);

  mongo.addNewReplSetMembers(db, addrToAdd, addrToRemove, true, function(err) {
    done(err);
  });
};

let podElection = function(pods) {
  //Because all the pods are going to be running this code independently, we need a way to consistently find the same
  //node to kick things off, the easiest way to do that is convert their ips into longs and find the highest
  pods.sort(function(a,b) {
    let aIpVal = ip.toLong(a.status.podIP);
    let bIpVal = ip.toLong(b.status.podIP);
    if (aIpVal < bIpVal) return -1;
    if (aIpVal > bIpVal) return 1;
    return 0; //Shouldn't get here... all pods should have different ips
  });

  //Are we the lucky one?
  return pods[0].status.podIP == hostIp;
};

let addrToAddLoop = function(pods, members) {
  let addrToAdd = [];
  for (let i in pods) {
    let pod = pods[i];
    if (pod.status.phase !== 'Running') {
      continue;
    }

    let podIpAddr = getPodIpAddressAndPort(pod);
    let podStableNetworkAddr = getPodStableNetworkAddressAndPort(pod);
    let podInRs = false;

    for (let j in members) {
      let member = members[j];
      if (member.name === podIpAddr || member.name === podStableNetworkAddr) {
        /* If we have the pod's ip or the stable network address already in the config, no need to read it. Checks both the pod IP and the
        * stable network ID - we don't want any duplicates - either one of the two is sufficient to consider the node present. */
        podInRs = true;
        break;
      }
    }

    if (!podInRs) {
      // If the node was not present, we prefer the stable network ID, if present.
      let addrToUse = podStableNetworkAddr || podIpAddr;
      addrToAdd.push(addrToUse);
    }
  }
  return addrToAdd;
};

let addrToRemoveLoop = function(members) {
    let addrToRemove = [];
    for (let i in members) {
        let member = members[i];
        if (memberShouldBeRemoved(member)) {
            addrToRemove.push(member.name);
        }
    }
    return addrToRemove;
};

let memberShouldBeRemoved = function(member) {
    return !member.health
        && moment().subtract(unhealthySeconds, 'seconds').isAfter(member.lastHeartbeatRecv);
};

/**
 * @param pod this is the Kubernetes pod, containing the info.
 * @returns string - podIp the pod's IP address with the port from config attached at the end. Example
 * WWW.XXX.YYY.ZZZ:27017. It returns undefined, if the data is insufficient to retrieve the IP address.
 */
let getPodIpAddressAndPort = function(pod) {
  if (!pod || !pod.status || !pod.status.podIP) {
    return;
  }

  return pod.status.podIP + ":" + config.mongoPort;
};

/**
 * Gets the pod's address. It can be either in the form of
 * '<pod-name>.<mongo-kubernetes-service>.<pod-namespace>.svc.cluster.local:<mongo-port>'. See:
 * <a href="https://kubernetes.io/docs/concepts/abstractions/controllers/statefulsets/#stable-network-id">Stateful Set documentation</a>
 * for more details. If those are not set, then simply the pod's IP is returned.
 * @param pod the Kubernetes pod, containing the information from the k8s client.
 * @returns string the k8s MongoDB stable network address, or undefined.
 */
let getPodStableNetworkAddressAndPort = function(pod) {
  if (!config.k8sMongoServiceName || !pod || !pod.metadata || !pod.metadata.name || !pod.metadata.namespace) {
    return;
  }

  let clusterDomain = config.k8sClusterDomain;
  let mongoPort = config.mongoPort;
  return pod.metadata.name + "." + config.k8sMongoServiceName + "." + pod.metadata.namespace + ".svc." + clusterDomain + ":" + mongoPort;
};

module.exports = {
  init: init,
  workloop: workloop
};
