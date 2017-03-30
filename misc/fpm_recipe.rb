#!/usr/bin/env ruby
$: << File.join(File.dirname(__FILE__), "..", "..", "lib")

# This example uses the API to create a package from local files
# it also creates necessary init-scripts and systemd files so our executable can be used as a service

require "fpm"
require "tmpdir"
require "fpm/package/pleaserun"

# enable logging
FPM::Util.send :module_function, :logger
FPM::Util.logger.level = :info
FPM::Util.logger.subscribe STDERR

if !ENV.key?('PACKAGE_DIR') || !ENV.key?('PACKAGE_VERSION')
  puts "PACKAGE_DIR and PACKAGE_VERSION should be set"
  exit 1
end

package = FPM::Package::Dir.new

package.attributes[:pleaserun_user] = "tsuru"
package.attributes[:pleaserun_group] = "tsuru"

# Set some attributes
package.name = 'tsuru-server'
package.version = ENV['PACKAGE_VERSION']
package.maintainer = 'tsuru@corp.globo.com'
package.vendor = 'Tsuru team <tsuru@corp.globo.com>'
package.url = 'https://tsuru.io'
package.description =  <<-EOS
tsuru-server is the server side of the Tsuru PaaS.

Tsuru is an open source platform as a service software. This package installs
the daemon used by application developers to communicate with tsuru server.
All jobs are disabled by default, to enable any of them, edit the file
/etc/default/tsuru-server.
EOS

# Add our files (should be in the current directory):
package.input("#{ENV['PACKAGE_DIR']}/tsurud=/usr/bin/")
package.input("./etc/tsuru.conf=/etc/tsuru/")

# Create two output packages!
pleaserun = package.convert(FPM::Package::PleaseRun)
pleaserun.input ["/usr/bin/tsurud", "api", "--config=/etc/tsuru/tsuru.conf"]
pleaserun.scripts[:after_install] = pleaserun.scripts[:after_install].gsub("#!/bin/sh","")
pleaserun.scripts[:after_install] = File.read('./misc/fpm_post_script.sh') + pleaserun.scripts[:after_install]
output_packages = []
output_packages << pleaserun.convert(FPM::Package::RPM)
output_packages << pleaserun.convert(FPM::Package::Deb)

# and write them both.
begin
  output_packages.each do |output_package|
    output = output_package.to_s
    output_package.output(output)

    puts "successfully created #{output}"
  end
ensure
  # defer cleanup until the end
  output_packages.each {|p| p.cleanup}
end
