let Client = require('node-kubernetes-client');
let config = require('./config');
let util = require('util');

fs = require('fs');

let readToken = fs.readFileSync('/var/run/secrets/kubernetes.io/serviceaccount/token');

let client = new Client({
  host: config.k8sROServiceAddress,
  namespace: config.namespace,
  protocol: 'https',
  version: 'v1',
  token: readToken
});

let getMongoPods = function getPods(done) {
  client.pods.get(function (err, podResult) {
    if (err) {
      return done(err);
    }
    let pods = [];
    for (let j in podResult) {
      pods = pods.concat(podResult[j].items)
    }
    let labels = config.mongoPodLabelCollection;
    let results = [];
    for (let i in pods) {
      let pod = pods[i];
      if (podContainsLabels(pod, labels)) {
        results.push(pod);
      }
    }

    done(null, results);
  });
};

let podContainsLabels = function podContainsLabels(pod, labels) {
  if (!pod.metadata || !pod.metadata.labels) return false;

  for (let i in labels) {
    let kvp = labels[i];
    if (!pod.metadata.labels[kvp.key] || pod.metadata.labels[kvp.key] != kvp.value) {
      return false;
    }
  }

  return true;
};

module.exports = {
  getMongoPods: getMongoPods
};
