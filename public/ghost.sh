#!/bin/bash
function usage() {
	prog=$(basename "$0")
	echo "Syntax: $prog <filename> [language]" >&2
	echo "        $prog -u <paste> <filename> [language]	- Update <paste>" >&2
	echo "        $prog -e <paste> [language]			- Edit <paste> in \$EDITOR (or vi.)" >&2
	echo "        $prog -d <paste>				- Delete <paste>" >&2
	echo "        $prog -s <paste>				- Show <paste>" >&2
	echo "        $prog -l					- List pastes" >&2
	echo "        $prog -U					- Upgrade ghost.sh (this will replace $0)" >&2
	echo "Options:" >&2
	echo "        -S <server>					- Override server" >&2
	echo "        -i						- Use http" >&2
	echo "        -I						- Use https, but disable certificate validation" >&2
	echo "        -F						- Force (upgrade, for example)" >&2
}

if [[ -z $1 ]]; then
	usage
	exit 1
fi

rcdir="${HOME}/.ghostbin"
if [[ ! -d "${rcdir}" ]]; then
	mkdir "${rcdir}"
fi


export -a curl_opts=("-c" "${rcdir}/cookie.jar" "-b" "${rcdir}/cookie.jar" "-f" "-s")

force=0
while getopts "S:iFIUhls:u:e:d:" o; do
	case $o in
		h)
			usage
			exit
			;;
		l)
			mode="list"
			;;
		s)
			mode="show"
			paste=$OPTARG
			;;
		u)
			mode="update"
			paste=$OPTARG
			;;
		e)
			mode="edit"
			paste=$OPTARG
			;;
		d)
			mode="delete"
			paste=$OPTARG
			;;
		U)
			mode="upgrade"
			;;
		F)
			force=1
			;;
		S)
			server="$OPTARG"
			;;
		i)
			proto="http"
			;;
		I)
			curl_opts+=("-k")
			;;
		?)
			usage
			exit 1
			;;
	esac
done
server=${server:-${proto:-https}://ghostbin.com}

# Look for a newer version of the script (but don't interrupt the user.)
upgrade=$(mktemp /tmp/ghost.XXXXXX)
{
	declare -a upg_curl_opts=("${curl_opts[@]}")
	[[ "$force" -eq 0 || "${mode}" != "upgrade" ]] && upg_curl_opts+=("-z" "$0")
	read -r code < <(curl "${upg_curl_opts[@]}" -w '%{http_code}' -o "${upgrade}" "${server}/ghost.sh");
	[[ $code -eq 200 ]] && echo "There's a new version of ghost.sh available at ${server}/ghost.sh" >&2
	if [[ $code -ne 200 ]]; then
		rm "${upgrade}"
		upgrade=
	fi
}

function _upgrade() {
	if [[ -z "${upgrade}" ]]; then
		echo "It doesn't get any better than this." >&2
		exit 1
	fi
	mv "${upgrade}" "${0}"
	chmod +x "${0}"
	echo "Done." >&2
	exit
}

[[ "${mode}" == "upgrade" ]] && _upgrade

[[ ! -z "${upgrade}" ]] && rm "${upgrade}"

shift $((OPTIND-1))

filename="$1"
lang="text"
if [[ ! -z $2 ]]; then
	lang=$2
fi

if [[ "${mode}" == "delete" ]]; then
	IFS='|' read -r code < <(curl "${curl_opts[@]}" -w '%{http_code}' --data-urlencode "(no body)" "${server}/paste/${paste}/delete")
	if [[ $code -ne 200 && $code -ne 303 && $code -ne 302 ]]; then
		echo "Rejected: $code" >&2
		exit 1
	fi
	echo "Deleted $paste."
	exit
elif [[ "${mode}" == "edit" ]]; then
	filename=$(mktemp /tmp/ghost.XXXXXX)
	lang=$1
	curl "${curl_opts[@]}" -o "${filename}" "${server}/paste/${paste}/raw"
	${EDITOR:-vi} "${filename}"
elif [[ "${mode}" == "show" ]]; then
	curl "${curl_opts[@]}" "${server}/paste/${paste}/raw"
	exit
elif [[ "${mode}" == "list" ]]; then
	IFS=' ' read -a pastes < <(curl "${curl_opts[@]}" "${server}/session/raw")
	for i in "${pastes[@]}"; do
		echo "$i: ${server}/paste/$i"
	done
	exit
fi

if [[ -z "${filename}" ]]; then
	usage
	exit 1
fi

pboard=
[[ -z "${pboard}" ]] && type pbcopy &> /dev/null && pboard=pbcopy
[[ -z "${pboard}" ]] && type xclip &> /dev/null && [[ -n "${DISPLAY}" ]] && pboard=xclip

url="${server}/paste/new"
[[ "${mode}" == "edit" || "${mode}" == "update" ]] && url="${server}/paste/${paste}/edit"

IFS='|' read -r code url < <(curl "${curl_opts[@]}" -w '%{http_code}|%{redirect_url}' --data-urlencode text@"$filename" ${lang:+--data-urlencode} ${lang:+lang="$lang"} "${url}" | sed -e 's/HTTP/http/g')
[[ "${mode}" == "edit" ]] && rm "${filename}"

if [[ $code -ne 200 && $code -ne 303 && $code -ne 302 ]]; then
	echo "Rejected: $code" >&2
	exit 1
fi
echo "$url"
[[ -n "${pboard}" ]] && (echo -n "$url" | $pboard; echo "Paste URL copied to clipboard." >&2)
