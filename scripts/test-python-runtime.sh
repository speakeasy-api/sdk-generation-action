#!/bin/sh
set -eu

python3 -c 'import sys; assert sys.version_info[:2] == (3, 12), sys.version'
go version
php --version

pipx install --global poetry==2.2.1
pipx install --global uv

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
cd "$test_dir"

cat > pyproject.toml <<'EOF'
[project]
name = "python-runtime-test"
version = "0.0.0"
requires-python = ">=3.10"
dependencies = ["pydantic==2.11.10"]

[tool.poetry]
package-mode = false

[dependency-groups]
dev = ["mypy==1.15.0", "pylint==3.2.3", "pyright==1.1.398"]
EOF

cat > check_runtime.py <<'EOF'
import sys

import pydantic_core
from pydantic import BaseModel

assert sys.version_info[:2] == (3, 12), sys.version
assert pydantic_core.__version__ == "2.33.2", pydantic_core.__version__


class Widget(BaseModel):
    name: str


assert Widget.model_validate_json('{"name":"example"}').name == "example"
EOF

poetry lock
poetry install --with dev
poetry run python check_runtime.py
poetry run pylint --version
poetry run mypy --version
poetry run pyright --version

rm -rf .venv
uv lock
uv sync --locked
uv run --locked python check_runtime.py
uv run --locked pylint --version
uv run --locked mypy --version
uv run --locked pyright --version
