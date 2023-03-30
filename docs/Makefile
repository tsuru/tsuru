.PHONY: html docs site

install-deps:
	@poetry --version &>/dev/null || python3 -m pip install poetry
	@poetry install -q

serve: install-deps
	@poetry run mkdocs serve
