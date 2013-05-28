#!/bin/bash
if [[ -z $1 ]]; then
	echo "Syntax: $0 <filename> [lang]" >&2
	exit 1
fi

rcdir="${HOME}/.ghostbin"
if [[ ! -d "${rcdir}" ]]; then
	mkdir "${rcdir}"
fi

# Look for a newer version of the script (but don't interrupt the user.)
(
	read -r code < <(curl -fs -w '%{http_code}' -z "$0" -o /dev/null http://ghostbin.com/ghost.sh);
	[[ $code -eq 200 ]] && echo "There's a new version of ghost.sh available at http://ghostbin.com/ghost.sh" >&2;
)

pboard=
[[ -z "${pboard}" ]] && type pbcopy &> /dev/null && pboard=pbcopy
[[ -z "${pboard}" ]] && type xclip &> /dev/null && [[ -n "${DISPLAY}" ]] && pboard=xclip

lang=text
if [[ ! -z $2 ]]; then
	lang=$2
fi
IFS='|' read -r code url < <(curl -c "${rcdir}/cookie.jar" -fs -w '%{http_code}|%{redirect_url}' --data-urlencode text@"$1" --data-urlencode lang="$lang" http://ghostbin.com/paste/new | sed -e 's/HTTP/http/g')
if [[ $code -ne 200 && $code -ne 303 && $code -ne 302 ]]; then
	echo "Rejected: $code" >&2
	exit 1
fi
echo "$url"
[[ -n "${pboard}" ]] && (echo -n "$url" | $pboard; echo "Paste URL copied to clipboard." >&2)
