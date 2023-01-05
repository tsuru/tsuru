#!/usr/bin/env bash
set -eu -o pipefail

[[ ${DEBUG:-no} != 'no' ]] && set -x

readonly DIFF=${DIFF:-diff}
readonly SWAGGERHUB=${SWAGGERHUB:-swaggerhub}
readonly YQ=${YQ:-yq}

readonly TSURU_ORGANIZATION=tsuru

extract_api_version() {
  local filename=${1}
  ${YQ} -r '.info.version' ${filename}
}

swaggerhub_api_get_version() {
  local api_name=${1}
  local destination=${2}

  { ${SWAGGERHUB} api:get ${api_name} 2> /dev/null || true; } > ${destination}
}

swaggerhub_api_create_version() {
  local api_name=${1}
  local filename=${2}

  ${SWAGGERHUB} api:create ${api_name} -f ${filename} \
    --setdefault --published=publish --visibility=public
}

swaggerhub_api_unpublish() {
  local api_name=${1}
  ${SWAGGERHUB} api:unpublish ${api_name}
}

swaggerhub_api_update_version() {
  local api_name=${1}
  local filename=${2}

  swaggerhub_api_unpublish ${api_name}

  ${SWAGGERHUB} api:update ${api_name} -f ${filename} \
    --published=publish
}

compare_api_specs() {
  local local=${1}
  local remote=${2}

  ${DIFF} -q \
    <(${YQ} -P 'sort_keys(..)' ${remote}) \
    <(${YQ} -P 'sort_keys(..)' ${local}) 2>&1 1> /dev/null
}

update_api_spec() {
  local api_namespace=${1}
  local local_api_spec_filename=${2}

  echo "Trying to update API ${api_namespace} with ${local_api_spec_filename} spec file"

  local api_version=$(extract_api_version ${local_api_spec_filename})
  local api_name="${api_namespace}/${api_version}"

  local remote_api_spec_filename=$(mktemp "remote-api-spec.XXXXX.yaml")

  swaggerhub_api_get_version ${api_name} ${remote_api_spec_filename}

  if [[ ! -s ${remote_api_spec_filename} ]]; then
    echo "API version (${api_name}) not found... trying to create a new one."
    swaggerhub_api_create_version ${api_name} ${local_api_spec_filename}
    echo "API version (${api_name}) created."
    rm ${remote_api_spec_filename}
    return 0
  fi

  echo "API version (${api_name}) found... trying to update it."

  set +e
  compare_api_specs ${local_api_spec_filename} ${remote_api_spec_filename}
  local has_changes=${?}
  set -e

  if [[ ${has_changes} -eq 0 ]]; then
    echo "No changes found between API spec local and remote... nothing to do."
    rm ${remote_api_spec_filename}
    return 0
  fi

  swaggerhub_api_update_version ${api_name} ${local_api_spec_filename}
  echo "API version (${api_name}) updated."
  rm ${remote_api_spec_filename}
}

main() {
  if [[ ${SWAGGERHUB_API_KEY:-n/a} == 'n/a' ]]; then
    echo 'Missing mandatory env var: SWAGGERHUB_API_KEY' 1>&2
    return 1
  fi

  declare -A api_spec_file_by_api_name=(
    [tsuru]="./docs/reference/api.yaml"
    [tsuru-router_api]="./docs/reference/router_api.yaml"
    [tsuru-service_api]="./docs/reference/service_api.yaml"
  )

  for api_name in ${!api_spec_file_by_api_name[@]}; do
    update_api_spec "${TSURU_ORGANIZATION}/${api_name}" "${api_spec_file_by_api_name[$api_name]}"
  done
}

main ${@}
