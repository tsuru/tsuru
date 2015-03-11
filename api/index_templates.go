// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"html/template"

	"github.com/tsuru/config"
)

var funcMap = template.FuncMap{
	"getConfig": func(v string) interface{} {
		result, _ := config.Get(v)
		return result
	},
}

var indexTemplate = template.Must(template.New("index").Funcs(funcMap).Parse(`
<html>
	<head>
		<meta charset="utf-8">
		<title>Welcome to tsuru!</title>
		<style>body {font-family: Helvetica, Arial;}</style>
	</head>
	<body>
		<h1>Welcome to tsuru!</h1>
		<p>tsuru is an open source PaaS, that aims to make it easier for developers to run their code in production.</p>
		<h2>Installing</h2>
		<p>Our documentation contains a guide for installing tsuru clients using package managers on Mac OS X, Ubuntu and ArchLinux, or build from source on any platform supported by Go: <a href="http://docs.tsuru.io/en/stable/using/install-client.html" title="Installing tsuru clients">docs.tsuru.io/en/stable/using/install-client.html</a>.</p>
		<p>Please ensure that you install the tsuru client, and then continue this guide with the configuration, user and team creation and the optional SSH key handling.</p>
		<h2>Configuring</h2>
		<p>In order to use this tsuru server, you need to add it to your set of targets:</p>
<pre>
$ tsuru target-add default {{.tsuruTarget}} -s
</pre>
		<p>tsuru supports multiple targets, the <code>-s</code> flag tells the client to add and set the given endpoint as the current target.</p>
		{{if .userCreate}}
		<h2>Create a user</h2>
		<p>After configuring the tsuru target that you wanna use, it's now needed to create a user:</p>
<pre>
$ tsuru user-create &lt;your-email&gt;
</pre>
		<p>The command will as for your password twice, and then register your user in the tsuru server.</p>
		<p>After creating your user, you need to authenticate with tsuru, using the <code>tsuru login</code> command.</p>
		{{else}}
		<h2>Login</h2>
		<p>Before using tsuru, you will need to ask an administrator to create a user for you, and the you will need to authenticate with your user, using the <code>tsuru login</code> command:</p>
		{{end}}
<pre>
$ tsuru login
</pre>
		{{if .nativeLogin}}
		<p>It will ask for your email and password, you can optionally provide your email as a parameter to the command.</p>
		{{else}}
		<p>It will use the OAuth provider for authenticating you with tsuru, opening the provider authentication URL in your browser.</p>
		{{end}}
		<h2>Ensure you're member of at least one team</h2>
		<p>In order to create an application, a user must be member of at least one team. You can see the teams that you are a member of by running the <code>team-list</code> command:</p>
<pre>
$ tsuru team-list
</pre>
		<p>If this command doesn't return any team for you, it means that you have to create a new team before creating your first application:</p>
<pre>
$ tsuru team-create &lt;team-name&gt;
</pre>
		{{if .keysEnabled}}
		<h2>Add an SSH key</h2>
		<p>In order to deploy your application using <code>git push</code>, you need to have an SSH key registered with tsuru, you can add a new SSH key using the <code>key-add</code> command:</p>
<pre>
$ tsuru key-add my-rsa-key ~/.ssh/id_rsa.pub
</pre>
		<p>Any key accepted by OpenSSH can be used with tsuru, this includes formats like RSA and DSA.</p>
		{{end}}
		<h2>Build and deploy your application</h2>
		<p>Now you're ready to deploy an application to this tsuru server, please refer to the tsuru documentation for more details: <a href="http://docs.tsuru.io/en/stable/using/python.html" title="Deploying Python applications in tsuru">docs.tsuru.io/en/stable/using/python.html</a>.</p>
	</body>
</html>
`))
