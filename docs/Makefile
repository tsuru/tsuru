.PHONY: html docs site

install-deps:
ifeq (, $(shell which poetry))
	python3 -m pip install poetry
endif
	@poetry install -q

serve: install-deps
	@if [ ! -f "docs/versions.json" ] ; then \
		echo '[{"version": ".", "title": "dev", "aliases": [""]},{"version": ".", "title": "latest", "aliases": []}]' > docs/versions.json ; \
		trap "trap - SIGINT SIGTERM ERR EXIT ; rm docs/versions.json; " SIGINT SIGTERM ERR EXIT ; \
	fi ; \
	poetry run mkdocs serve
