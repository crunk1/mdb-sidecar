let Db = require('mongodb').Db;
let MongoServer = require('mongodb').Server;
let async = require('async');
let config = require('./config');

let localhost = '127.0.0.1'; //Can access mongo as localhost from a sidecar

let getDb = function(host, done) {
  //If they called without host like getDb(function(err, db) { ... });
  if (arguments.length === 1) {
    if (typeof arguments[0] === 'function') {
      done = arguments[0];
      host = localhost;
    } else {
      throw new Error('getDb illegal invocation. User either getDb(\'options\', function(err, db) { ... }) OR getDb(function(err, db) { ... })');
    }
  }

  let mongoOptions = {};
  host = host || localhost;

  if (config.mongoSSLEnabled) {
    mongoOptions = {
      ssl: config.mongoSSLEnabled,
      sslAllowInvalidCertificates: config.mongoSSLAllowInvalidCertificates,
      sslAllowInvalidHostnames: config.mongoSSLAllowInvalidHostnames
    }
  }

  let mongoDb = new Db(config.database, new MongoServer(host, config.mongoPort, mongoOptions));

  mongoDb.open(function (err, db) {
    if (err) {
      return done(err);
    }

    if(config.username) {
        mongoDb.authenticate(config.username, config.password, function(err, result) {
            if (err) {
              return done(err);
            }

            return done(null, db);
        });
    } else {
      return done(null, db);
    }

  });
};

let replSetGetConfig = function(db, done) {
  db.admin().command({ replSetGetConfig: 1 }, {}, function (err, results) {
    if (err) {
      return done(err);
    }

    return done(null, results.config);
  });
};

let replSetGetStatus = function(db, done) {
  db.admin().command({ replSetGetStatus: {} }, {}, function (err, results) {
    if (err) {
      return done(err);
    }

    return done(null, results);
  });
};

let initReplSet = function(db, hostIpAndPort, done) {
  console.log('initReplSet', hostIpAndPort);

  db.admin().command({ replSetInitiate: {} }, {}, function (err) {
    if (err) {
      return done(err);
    }

    //We need to hack in the fix where the host is set to the hostname which isn't reachable from other hosts
    replSetGetConfig(db, function(err, rsConfig) {
      if (err) {
        return done(err);
      }

      console.log('initial rsConfig is', rsConfig);
      rsConfig.configsvr = config.isConfigRS;
      rsConfig.members[0].host = hostIpAndPort;
      async.retry({times: 20, interval: 500}, function(callback) {
        replSetReconfig(db, rsConfig, false, callback);
      }, function(err, results) {
        if (err) {
          return done(err);
        }

        return done();
      });
    });
  });
};

let replSetReconfig = function(db, rsConfig, force, done) {
  console.log('replSetReconfig', rsConfig);

  rsConfig.version++;

  db.admin().command({ replSetReconfig: rsConfig, force: force }, {}, function (err) {
    if (err) {
      return done(err);
    }

    return done();
  });
};

let addNewReplSetMembers = function(db, addrToAdd, addrToRemove, shouldForce, done) {
  replSetGetConfig(db, function(err, rsConfig) {
    if (err) {
      return done(err);
    }

    removeDeadMembers(rsConfig, addrToRemove);

    addNewMembers(rsConfig, addrToAdd);

    replSetReconfig(db, rsConfig, shouldForce, done);
  });
};

let addNewMembers = function(rsConfig, addrsToAdd) {
  if (!addrsToAdd || !addrsToAdd.length) return;

  let memberIds = [];
  let newMemberId = 0;

  // Build a list of existing rs member IDs
  for (let i in rsConfig.members) {
    memberIds.push(rsConfig.members[i]._id);
  }

  for (let i in addrsToAdd) {
    let addrToAdd = addrsToAdd[i];

    // Search for the next available member ID (max 255)
    for (let i = newMemberId; i <= 255; i++) {
      if (!memberIds.includes(i)) {
        newMemberId = i;
        memberIds.push(newMemberId);
        break;
      }
    }

    // Somehow we can get a race condition where the member config has been updated since we created the list of
    // addresses to add (addrsToAdd) ... so do another loop to make sure we're not adding duplicates
    let exists = false;
    for (let j in rsConfig.members) {
      let member = rsConfig.members[j];
      if (member.host === addrToAdd) {
        console.log("Host [%s] already exists in the Replicaset. Not adding...", addrToAdd);
        exists = true;
        break;
      }
    }

    if (exists) {
      continue;
    }

    let cfg = {
      _id: newMemberId,
      host: addrToAdd
    };

    rsConfig.members.push(cfg);
  }
};

let removeDeadMembers = function(rsConfig, addrsToRemove) {
  if (!addrsToRemove || !addrsToRemove.length) return;

  for (let i in addrsToRemove) {
    let addrToRemove = addrsToRemove[i];
    for (let j in rsConfig.members) {
      let member = rsConfig.members[j];
      if (member.host === addrToRemove) {
        rsConfig.members.splice(j, 1);
        break;
      }
    }
  }
};

let isInReplSet = function(ip, done) {
  getDb(ip, function(err, db) {
    if (err) {
      return done(err);
    }

    replSetGetConfig(db, function(err, rsConfig) {
      db.close();
      if (!err && rsConfig) {
        done(null, true);
      }
      else {
        done(null, false);
      }
    });
  });
};

module.exports = {
  getDb: getDb,
  replSetGetStatus: replSetGetStatus,
  initReplSet: initReplSet,
  addNewReplSetMembers: addNewReplSetMembers,
  isInReplSet: isInReplSet
};
