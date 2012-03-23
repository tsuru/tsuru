CREATE TABLE 'services' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_type_id' integer,'name' varchar(255));
CREATE TABLE 'service_types' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'charm' varchar(255));
CREATE TABLE 'service_apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'service_id' integer, 'app_id' integer);
CREATE TABLE 'apps' ('id' INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, 'name' varchar(255), 'framework' varchar(255), 'state' varchar(255), 'ip' varchar('100'));

INSERT INTO service_types (name, charm) VALUES ("Mysql", "mysql");
