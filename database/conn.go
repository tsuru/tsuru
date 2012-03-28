package database

import (
	"database/sql"
	"launchpad.net/mgo"
)

var Db *sql.DB
var Session *mgo.Session
