#!/usr/bin/env bash
#
# local-dev.sh - Local development CLI tool used to run Tsuru.
#
# Version: 0.1.0


### Constants
readonly FAKE_HOST_IP="100.64.100.100"
readonly TSURU_API_PORT="8080"
readonly TEMPLATES_DIR="./etc"


# ------------------------------------------------------------------------------
# Pre-flight checks
#
# These functions are used to check if the system is ready to run the script.
# ------------------------------------------------------------------------------
preflight_check_deps() {
    local deps=("$@")

    for dep in "${deps[@]}"; do
        if ! command -v "${dep}" &>/dev/null; then
            echo "Failed to find dependency: ${dep}"
            exit 1
        fi
    done
}

preflight_checks() {
    # Chech common dependencies
    preflight_check_deps "docker" "envsubst" "tsuru"

    # Check OS specific dependencies
    case "$(uname -s)" in
        Darwin)
            preflight_check_deps "ifconfig"
            ;;

        Linux)
            preflight_check_deps "ip"
            ;;

        *)
            echo "Unsupported OS: $(uname -s)"
            exit 1
            ;;
    esac
}


# ------------------------------------------------------------------------------
# Utility functions
#
# These functions are used to perform common tasks that are used by the command
# execution functions.
# ------------------------------------------------------------------------------
get_ifname() {
    case "$(uname -s)" in
        Darwin) echo lo0 ;;
        Linux) echo lo ;;
    esac
}

ifname_has_ip() {
    local interface_name=$1
    local ip=$2

    case "$(uname -s)" in
        Darwin)
            ifconfig "${interface_name}" | grep -q "${ip}"
            return $?
            ;;

        Linux)
            ip addr show "${interface_name}" | grep -q "${ip}"
            return $?
            ;;
    esac
}

ifname_add_ip() {
    local interface_name=${1}
    local ip=${2}

    case "$(uname -s)" in
        Darwin)
            sudo ifconfig "${interface_name}" alias "${ip}/32"
            ;;	

        Linux)
            sudo ip addr add "${ip}/32" dev "${interface_name}"
            ;;
    esac
}

render_config_template() {
    local src=$1
    local dst=$2
    local fake_host_ip=$3
    local host_port=${4:-"$TSURU_API_PORT"}

    {
        TSURU_HOST_IP="$fake_host_ip" TSURU_HOST_PORT="$host_port" \
        envsubst < "$src" > "$dst"
    }
}


# ------------------------------------------------------------------------------
# Command execution functions
#
# These functions are the actual commands that are executed when the script is
# run with a specific command.
# ------------------------------------------------------------------------------
exec_setup_loopback() {
    local fake_host_ip=${1:-"$FAKE_HOST_IP"}
    local ifname=$(get_ifname)

    echo "Checking if the IP $fake_host_ip is assigned to the interface $ifname..."
    ifname_has_ip "$ifname" "$fake_host_ip"
    if [ $? -ne 0 ]; then
        echo "Assigning the IP $fake_host_ip to the interface $ifname..."
        ifname_add_ip "$ifname" "$fake_host_ip"
    fi
}

exec_setup_tsuru_user() {
    local user=${1:-"admin@admin.com"}
    local password=${2:-"admin@123"}

    echo "Setting up the root user with email $user and password $password..."

    # Create the root user in Tsuru
    # Ignore the output and errors because the user may already exist
    echo -e "${password}\n${password}\n" | 
        docker exec -i tsuru-api tsurud root user create "$user" &> /dev/null || true
}

exec_setup_tsuru_target() {
    local target_host=${1:-"$FAKE_HOST_IP"}
    local target_port=${2:-"$TSURU_API_PORT"}
    local target_name=${3:-"local-dev"}

    # Add the Tsuru target, ignore the output and errors because the target may already exist
    echo "Setting up the Tsuru target $target_name at http://${target_host}:${target_port}..."
    tsuru target add "$target_name" "http://${target_host}:${target_port}" &> /dev/null || true
}

exec_setup_tsuru_cluster() {
    local tsuru_host_ip=${1:-"$FAKE_HOST_IP"}
    local kconfig=${KUBECONFIG:-"$HOME/.kube/config"}

    kconfig=$(echo $kconfig | cut -d: -f1)

    tsuru --target=local-dev cluster list | grep -q my-cluster
    if [ $? -eq 0 ]; then
        echo "Cluster my-cluster already exists, skipping..."
        return
    fi

    tsuru --target=local-dev cluster add my-cluster kubernetes \
        --addr       $(yq -r '.clusters[] | select(.name == "minikube") | .cluster.server' ${kconfig}) \
        --cacert     $(yq -r '.clusters[] | select(.name == "minikube") | .cluster["certificate-authority"]' ${kconfig}) \
        --clientcert $(yq -r '.users[] | select(.name == "minikube") | .user["client-certificate"]' ${kconfig}) \
        --clientkey  $(yq -r '.users[] | select(.name == "minikube") | .user["client-key"]' ${kconfig}) \
        --custom "registry=${tsuru_host_ip}:5000/tsuru" \
        --custom "registry-insecure=true" \
        --custom "build-service-address=dns:///${tsuru_host_ip}:8000" \
        --custom "build-service-tls=false" \
        --default
}

exec_render_templates() {
    local fake_host_ip=${1:-"$FAKE_HOST_IP"}

    for template_path in $(find ${TEMPLATES_DIR}/*.template); do
        local destination_path=${template_path%.template}

        echo "Redering template file ${template_path} at ${destination_path}..."
        render_config_template "${template_path}" "${destination_path}" "${fake_host_ip}"
    done
}

exec_help() {
    echo "Usage: $(basename $0) [COMMAND|OPTIONS]"
    echo
    echo "COMMANDS:"
    echo "  setup-loopback      Setup the loopback interface with a fake IP"
    echo "  setup-tsuru-user     Setup the root user in Tsuru"
    echo "  render-templates    Render the configuration templates"
    echo
    echo "OPTIONS:"
    echo "  -h, --help      Print this help message"
    echo "  -v, --version   Print current version"
}

exec_version() {
    grep '^# Version: ' "$0" | cut -d ':' -f 2 | tr -d ' '
}


# ------------------------------------------------------------------------------
# Main execution
#
# This is the main execution of the script. It will parse the command line
# arguments and execute the appropriate command.
# ------------------------------------------------------------------------------
[ -n "${DEBUG}" ] && set -x

if [ $# -eq 0 ]; then
    exec_help
    exit 1
fi

case "$1" in
    # commands
    setup-loopback      ) exec_setup_loopback     "${@:2}" ;;
    setup-tsuru-user    ) exec_setup_tsuru_user   "${@:2}" ;;
    setup-tsuru-target  ) exec_setup_tsuru_target "${@:2}" ;;
    setup-tsuru-cluster ) exec_setup_tsuru_cluster "${@:2}" ;;
    render-templates    ) exec_render_templates   "${@:2}" ;;

    # options
    -h | --help    ) exec_help     ;;
    -v | --version ) exec_version  ;;

    *)
        echo "Unknown command: $1"
        exec_help
        exit 1
        ;;
esac
