.PHONY: html docs site

install-deps:
	@poetry --version &>/dev/null || python3 -m pip install poetry
	@poetry install -q

serve: install-deps
	@if [ ! -f "docs/versions.json" ] ; then \
		echo '[{"version": ".", "title": "dev", "aliases": [""]},{"version": ".", "title": "latest", "aliases": []}]' > docs/versions.json ; \
		trap "trap - SIGINT SIGTERM ERR EXIT ; rm docs/versions.json; " SIGINT SIGTERM ERR EXIT ; \
	fi ; \
	poetry run mkdocs serve
