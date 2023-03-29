.PHONY: html docs site
BUILDDIR ?= .
virtualenv_dir = .venv
venv = . $(virtualenv_dir)/bin/activate

$(virtualenv_dir):
	@echo "===== Creating virtualenv... ====="
	@python3 -m pip show virtualenv &>/dev/null || python3 -m pip install virtualenv
	@python3 -m virtualenv .venv
	@echo "===== Virtualenv created. ====="

install-deps: $(virtualenv_dir)
	@$(venv) ; \
		pip -vvv freeze -r requirements.txt 2>&1 \
			| grep "not installed" &>/dev/null \
		&& { \
			echo "===== Installing dependencies... =====" ; \
			pip install -r requirements.txt ; \
			echo "===== Dependencies Installed =====" ; \
		}	\
		|| : "All dependencies are already installed"

html: install-deps
	@echo "===== Building HTML files... ====="
	@$(venv) ; mkdocs build --clean
	@echo "===== Build finished. The HTML pages are in $(BUILDDIR)/site. ====="

serve: install-deps
	@$(venv) ; mkdocs serve


