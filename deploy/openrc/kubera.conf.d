# /etc/conf.d/kubera — environment for the Kubera OpenRC service.
# Must use export: the daemon inherits only exported variables.

export KUBERA_DATABASE_PATH=/var/lib/kubera/kubera.db
export KUBERA_LOG_LEVEL=info
export KUBERA_BUSY_TIMEOUT_MS=5000
export KUBERA_MIGRATE_ON_START=true

# Optional: override the FIFO path that keeps stdin open.
#export KUBERA_STDIN=/run/kubera.stdin
