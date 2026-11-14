// Loads the browser's crypto.js (no browser needed) and prints NHC.passwordKeys(password, salt).write.
// Used by internal/client/client_test.go to prove the browser and the Go client derive the same proof.
// usage: node scripts/check-crypto-vector.js <password> <salt>
'use strict';
const fs = require('fs');
const path = require('path');
const { webcrypto } = require('crypto');

const win = { crypto: webcrypto };
const src = fs.readFileSync(path.join(__dirname, '..', 'web', 'static', 'js', 'crypto.js'), 'utf8');
new Function('window', src).call(win, win);
win.NHC.passwordKeys(process.argv[2], process.argv[3]).then(function (k) {
  process.stdout.write(k.write + '\n');
}, function (e) {
  process.stderr.write(String(e) + '\n');
  process.exit(1);
});
