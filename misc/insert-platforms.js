// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This script register some platforms in the database.
//
// Usage, from the command line:
//
//   % mongo <database-name> misc/insert-platforms.js

var platforms = ["nodejs", "php", "python", "ruby", "static"];
for(var i = 0; i < platforms.length; i++) {
	db.platforms.insert({id: platforms[i]});
}
