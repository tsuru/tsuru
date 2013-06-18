var platforms = ["nodejs", "php", "python", "ruby", "static"];
for(var i in platforms) {
	db.platforms.insert({id: platforms[i]});
}
