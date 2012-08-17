/*
tsuru is a command line tool for application developers.

It provide some commands that allow a developer to register himself/herself,
manage teams, apps and services.

Usage:

	% tsuru <command> [args]

The currently available commands are (grouped by subject):

    target            changes or retrive the current tsuru server

    user-create       creates a new user
    login             authenticates the user with tsuru server
    logout            finishes the session with tsuru server
    key-add           adds a public key to tsuru deploy server
    key-remove        removes a public key from tsuru deploy server

    team-create       creates a new team (adding the current user to it automatically)
    team-list         list teams that the user is member
    team-user-add     adds a user to a team
    team-user-remove  removes a user from a team

    app-create        creates an app
    app-remove        removes an app
    app-list          lists apps that the user has access (see app-grant and team-user-add)
    app-grant         allows a team to have access to an app
    app-revoke        revokes access to an app from a team
    log               shows log for an app
    run               runs a command in all units of an app

    env-get           display environment variables for an app
    env-set           set environment variable(s) to an app
    env-unset         unset environment variable(s) from an app

    bind              binds an app to a service instance
    unbind            unbinds an app from a service instance

    service-list      list all services, and instances of each service
    service-add       creates a new instance of a service
    service-remove    removes a instance of a service
    service-status    checks the status of a service instance
    service-info      list instances of a service, and apps binded to each instance
    service-doc       displays documentation for a service

Use "tsuru help <command>" for more information about a command.
*/
package documentation
