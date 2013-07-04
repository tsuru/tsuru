function addPlatforms() {
    var platforms = new Array(
        {"_id": "static"},
        {"_id": "python"},
        {"_id": "ruby"},
        {"_id": "nodejs"},
        {"_id": "php"}
    );
    for (var i=0; i<platforms.length; i++) {
        db["platforms"].save(platforms[i]);
    }
}

addPlatforms();
