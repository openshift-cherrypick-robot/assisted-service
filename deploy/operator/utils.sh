set -o nounset
set -o pipefail
set -o errexit

function wait_for_crd() {
    crd="$1"
    namespace="${2:-}"

    wait_for_condition "crd/${crd}" "Established" "60s" "${namespace}"
}

function wait_for_operator() {
    subscription="$1"
    namespace="${2:-}"
    echo "Waiting for operator ${subscription} to get installed on namespace ${namespace}..."

    for _ in $(seq 1 60); do
        csv=$(oc -n "${namespace}" get subscription "${subscription}" -o jsonpath='{.status.installedCSV}' || true)
        if [[ -n "${csv}" ]]; then
            if [[ "$(oc -n "${namespace}" get csv "${csv}" -o jsonpath='{.status.phase}')" == "Succeeded" ]]; then
                echo "ClusterServiceVersion (${csv}) is ready"
                return 0
            fi
        fi

        sleep 10
    done

    echo "Timed out waiting for csv to become ready!"
    return 1
}

function wait_for_pod() {
    pod="$1"
    namespace="${2:-}"
    selector="${3:-}"

    wait_for_condition "pod" "Ready" "22m" "${namespace}" "${selector}"
}

function hash() {
    input=$1
    length=$2
    echo "${input}" | md5sum - | cut -c -${length}
}

function disk_to_wwn() {
    # The WWN token has to begin with a number, so it was prefixed with an
    # hardcoded 0 and afterwards 15 bytes of a digested hash from the original
    # disk name.
    echo "0x0$(hash ${1} 15)"
}

function wait_for_condition() {
    object="$1"
    condition="$2"
    timeout="$3"
    namespace="${4:-}"
    selector="${5:-}"

    echo "Waiting for (${object}) on namespace (${namespace}) with labels (${selector}) to be created..."
    for i in {1..40}; do
        oc get ${object} --selector="${selector}" --namespace=${namespace} |& grep -ivE "(no resources found|not found)" && break || sleep 10
    done

    echo "Waiting for (${object}) on namespace (${namespace}) with labels (${selector}) to become (${condition})..."
    oc wait -n "${namespace}" --for=condition=${condition} --selector "${selector}" ${object} --timeout=${timeout}
}

function wait_for_object_amount() {
    object="$1"
    amount="$2"
    interval="$3"
    namespace="${4:-}"

    until [ $(oc get ${object} -n "${namespace}" --no-headers | wc -l) -eq ${amount} ]; do
        sleep ${interval}
    done
    echo "done" $(oc get ${object} -n "${namespace}" --no-headers | wc -l)
}

function wait_for_boolean_field() {
    object="$1"
    field="$2"
    namespace="${3:-}"
    interval="${4:-10}"

    for i in {1..10}; do
        value=$(oc get -n ${namespace} ${object} -o custom-columns=field:${field} --no-headers)
        if [ "${value}" = "true" ]; then
            return 0
        fi

        sleep ${interval}
    done

    echo "Value of field ${field} of object ${object} under namespace ${namespace} has never become true"
    return 1
}

function get_image_without_tag() {
    # given "<registry>/<repository>/<project>:<tag>"
    # return "<registry>/<repository>/<project>"
    echo "${1}" | cut -d':' -f1 | cut -d'@' -f1
}

function get_image_namespace() {
    # given "<registry>/<repository>/<project>:<tag>"
    # return "<registry>/<repository>"
    dirname "${1}"
}

function get_image_without_registry() {
    # given "<registry>/<repository>/<project>:<tag>"
    # return "<repository>/<project>:<tag>"
    echo "${1#*/}"
}

function get_image_namespace_without_registry() {
    # given "<registry>/<repository>/<project>:<tag>"
    # return "<repository>"
    get_image_namespace $(get_image_without_registry "${1}")
}

function get_image_repository_only() {
    # given "<registry>/<repository>/<project>:<tag>"
    # return "<repository>/<project>"
    get_image_without_registry $(get_image_without_tag "${1}")
}

function nth_ip() {
  network=$1
  idx=$2

  python -c "from ansible_collections.ansible.netcommon.plugins.filter import ipaddr; print(ipaddr.nthhost('"$network"', $idx))"
}

function retry() {
    attempts=5
    interval=1

    local OPTIND
    while getopts "a:i:" opt ; do
      case "${opt}" in
          a )
              attempts="${OPTARG}"
              ;;
          i )
              interval="${OPTARG}"
              ;;
          * )
              ;;
      esac
    done
    shift $((OPTIND-1))

    rc=0
    for attempt in $(seq "${attempts}")
    do
        echo "Attempt ${attempt}/${attempts} to execute \"$*\"..."

        "$@"
        rc=$?
        if [[ ${rc} = 0 ]]
        then
            break
        fi

        sleep "${interval}"
    done

    return ${rc}
}
