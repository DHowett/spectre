#!/bin/bash
VERSION=1.0

function usage() {
	prog=$(basename "$0")
	echo "Syntax: $prog [-p] <filename> [language]" >&2
	echo "        $prog -u <paste> <filename> [language]	- Update <paste>" >&2
	echo "        $prog -e <paste> [language]			- Edit <paste> in \$EDITOR (or vi.)" >&2
	echo "        $prog -d <paste>				- Delete <paste>" >&2
	echo "        $prog -s <paste>				- Show <paste>" >&2
	echo "        $prog -l					- List pastes" >&2
	echo "        $prog -U					- Upgrade ghost.sh (this will replace $0)" >&2
	echo "Options:" >&2
	echo "        -x <expiry>					- Expiration for paste (with units: ns/us/ms/s/m/h)" >&2
	echo "        -p						- Prompt for password" >&2
	echo "        -S <server>					- Override server" >&2
	echo "        -i						- Use http" >&2
	echo "        -I						- Use https, but disable certificate validation" >&2
	echo "        -F						- Force (upgrade, for example)" >&2
	echo "        -L						- Request Login" >&2
}

if [[ -z $1 ]]; then
	usage
	exit 1
fi

rcdir="${HOME}/.ghostbin"
if [[ ! -d "${rcdir}" ]]; then
	mkdir "${rcdir}"
fi


export -a curl_opts=("-c" "${rcdir}/cookie.jar" "-b" "${rcdir}/cookie.jar" "-A" "ghost.sh/${VERSION}" "-f" "-s")

force=0
passworded=0

while getopts "d:e:FhIiLlpS:s:t:Uu:x:" o; do
	case $o in
		d)
			mode="delete"
			paste=$OPTARG
			;;
		e)
			mode="edit"
			paste=$OPTARG
			;;
		F)
			force=1
			;;
		h)
			usage
			exit
			;;
		I)
			curl_opts+=("-k")
			;;
		i)
			proto="http"
			;;
		L)
			mode="login"
			;;
		l)
			mode="list"
			;;
		p)
			passworded=1
			;;
		S)
			server="$OPTARG"
			;;
		s)
			mode="show"
			paste=$OPTARG
			;;
		U)
			mode="upgrade"
			;;
		u)
			mode="update"
			paste=$OPTARG
			;;
		x)
			expiry=$OPTARG
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

function _mode() {
	# GNU syntax first, BSD second. BSD stat is more forgiving of errors.
	stat -c '%a' $1 2>/dev/null || stat -f '%Lp' $1 2>/dev/null
}

function _upgrade() {
	if [[ -z "${upgrade}" ]]; then
		echo "It doesn't get any better than this." >&2
		exit 1
	fi
	chmod "$(_mode "${0}")" "${upgrade}"
	mv "${upgrade}" "${0}"
	echo "Done." >&2
	exit
}

function _password() {
	read -p "Password:" -r -s $1 < /dev/tty
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
elif [[ "${mode}" == "login" ]]; then
	url="${server}/auth/token"
	IFS='|' read -r code url < <(curl "${curl_opts[@]}" -w '%{http_code}|%{redirect_url}' "${url}" | sed -e 's/HTTP/http/g')
	if [[ $code -ne 200 && $code -ne 303 && $code -ne 302 ]]; then
		echo "Rejected: $code" >&2
		exit 1
	fi

	token=${url##*/}

	echo "To log in, please visit $url" >&2

	type open &> /dev/null && open "$url"
	type xdg-open &> /dev/null && xdg-open "$url"

	echo "" >&2
	echo "(waiting for login)" >&2
	{
		l=0
		trap "l=-1" 2       # INT
		filename=$(mktemp /tmp/ghost.XXXXXX)
		while [[ $l -eq 0 ]]; do
			sleep 2
			IFS='|' read -r code < <(curl -w '%{http_code}' -o "${filename}" "${curl_opts[@]}" --data-urlencode "type=token" --data-urlencode "token=${token}" "${server}/auth/login")
			if [[ $code -ne 200 && $code -ne 418 ]]; then
				echo "" >&2
				echo "Login Rejected. Detailed response follows." >&2
				cat "${filename}" >&2
				echo "" >&2
				l=-1
			elif [[ $code -eq 200 ]]; then
				echo "" >&2
				echo "Success!" >&2
				l=1
			else
				printf "."
			fi
		done
		rm -f "${filename}"
	}
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

[[ $passworded -eq 1 ]] && _password pw && echo
export -a curl_formargs=("--data-urlencode" "text@$filename")
[[ ! -z "${lang}" ]]	&& curl_formargs+=("--data-urlencode" "lang=${lang}")
[[ ! -z "${pw}" ]]	&& curl_formargs+=("--data-urlencode" "password=${pw}")
[[ ! -z "${expiry}" ]]	&& curl_formargs+=("--data-urlencode" "expire=${expiry}")

IFS='|' read -r code url < <(curl "${curl_opts[@]}" -w '%{http_code}|%{redirect_url}' "${curl_formargs[@]}" "${url}" | sed -e 's/HTTP/http/g')
[[ "${mode}" == "edit" ]] && rm "${filename}"

if [[ $code -ne 200 && $code -ne 303 && $code -ne 302 ]]; then
	echo "Rejected: $code" >&2
	exit 1
fi
echo "$url"
[[ -n "${pboard}" ]] && (echo -n "$url" | $pboard; echo "Paste URL copied to clipboard." >&2)
