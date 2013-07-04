function addPlatforms() {
    var platforms = new Array(
        {"name": "static"},
        {"name": "python"},
        {"name": "ruby"},
        {"name": "nodejs"},
        {"name": "php"}
    );
    for (var i=0; i<platforms.length; i++) {
        db["platforms"].save(platforms[i]);
    }
}

addPlatforms();
