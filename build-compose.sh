#!/bin/bash

# This script adds a fake IP used as the IP of Tsuru API while running it on Docker Compose.

set -eu -o pipefail

readonly FAKE_HOST_IP=${FAKE_HOST_IP:-100.64.100.100}
readonly INTERFACE_NAME=${INTERFACE_NAME:-auto-detect} # the interface to assign the fake host IP (defaults to loopback interface)
readonly TEMPLATES_DIR=./etc

readonly DEBUG=${DEBUG:-}

[[ -n ${DEBUG} ]] && set -x

function get_loopback_interface_name() {
  local os_name=${1}

  case ${os_name} in
    Darwin)
      echo lo0;;

    *)
      echo lo;;
  esac
}

function set_ip_on_interface() {
  local os_name=${1}
  local interface_name=${2}
  local ip=${3}

  if [[ $(command -v ifconfig) ]]; then
    sudo ifconfig "${interface_name}" alias "${FAKE_HOST_IP}/32"
    return $?
  fi

  echo "ifconfig not found" >&2
  exit 2
}

function replace_ip_on_templates() {
  local src=${1}
  local dst=${2}

  {
    HOST_IP=${FAKE_HOST_IP} \
    TSURU_HOST_IP=${FAKE_HOST_IP} \
      envsubst < ${src} > ${dst};
  }
}

function main() {
  local os_name=$(uname)

  local interface_name=${INTERFACE_NAME}
  if [[ ${INTERFACE_NAME} == "auto-detect" ]]; then
    interface_name=$(get_loopback_interface_name ${os_name})
  fi

  echo "Assigning the ${FAKE_HOST_IP} IP on ${interface_name} interface"
  set_ip_on_interface ${os_name} ${interface_name} ${FAKE_HOST_IP}

  for template_path in $(find ${TEMPLATES_DIR}/*.template); do
    local destination_path=${template_path%.template}

    echo "Redering template file ${template_path} at ${destination_path}..."
    replace_ip_on_templates ${template_path} ${destination_path}
  done
}

main $@
