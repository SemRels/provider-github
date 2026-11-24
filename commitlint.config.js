// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The semrel Authors
// yaml-language-server: $schema=https://json.schemastore.org/commitlintrc.json
module.exports = {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "signed-off-by": [
      2,
      "always",
      "Signed-off-by:",
    ],
    "type-enum": [
      2,
      "always",
      [
        "feat",
        "fix",
        "docs",
        "style",
        "refactor",
        "perf",
        "test",
        "build",
        "ci",
        "chore",
        "revert",
        "major",
        "minor",
        "patch",
        "deps",
      ],
    ],
  },
};
