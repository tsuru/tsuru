CREATE TABLE services (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, service_type_id integer,name varchar(255));
CREATE TABLE service_types (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name varchar(255), charm varchar(255));
CREATE TABLE service_apps (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, service_id integer, app_id integer);
CREATE TABLE apps (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name varchar(255), framework varchar(255), state varchar(255), ip varchar(100));
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, email VARCHAR(255) UNIQUE, password VARCHAR(255));
CREATE TABLE usertokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    user_id INTEGER NOT NULL,
    token VARCHAR(255) NOT NULL,
    valid_until TIMESTAMP NOT NULL,

    CONSTRAINT fk_tokens_users FOREIGN KEY(user_id) REFERENCES users(id)
);

INSERT INTO service_types (name, charm) VALUES ("Mysql", "mysql");
