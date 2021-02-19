Various monitoring plugins in golang
=====================================

This repository is meant to host various monitoring plugins for Nagios (or
Centreon, or Icinga) written in golang. I will update it as I rewrite my old
shell-based scripts or write new ones.

Building
---------

Running the `build.sh` bash script will build the plugins for both the `amd64`
and `386` architectures. It will create a `bin/` directory with
architecture-specific subdirectories.

Plugins
--------

### SSL certificate expiry

The `check_ssl_certificate` plugin can be used to check that the certificate
from a TLS service has not expired and is not going to expire shortly. It
supports the following command-line flags:

* `-H name`/`--hostname name`: the host name to connect to.
* `-P port`/`--port port`: the TCP port to connect to.
* `-W days`/`--warning days`: a threshold, in days, below which a warning will
  be emitted for this service.
* `-C days`/`--critical days`: a threshold, in days, below which the plugin will
  indicate that the service is in a critical state.
* `--ignore-cn-only`: do not cause errors if a certificate does not have SANs
  and relies on the CN field.
* `-a names`/`--additional-names names`: a comma-separated list of DNS names
  that the certificate should also have.
* `-s protocol`/`--start-tls protocol`: protocol to use before requesting a
  switch to TLS. Supported protocols: `smtp`, `sieve`.