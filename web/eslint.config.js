// Flat config. React-specific rules arrive with the app itself (TASKS.md 6.1).
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import prettier from "eslint-config-prettier";
import globals from "globals";

export default tseslint.config(
  {
    // Generated code is not ours to lint (AGENTS.md §9).
    ignores: ["dist/**", "dev-dist/**", "src/gen/**", "node_modules/**"],
  },
  js.configs.recommended,
  tseslint.configs.strictTypeChecked,
  tseslint.configs.stylisticTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
      globals: { ...globals.browser },
    },
    rules: {
      // `unknown` + narrowing is the escape hatch, never `any` (AGENTS.md §7).
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/consistent-type-imports": [
        "error",
        { fixStyle: "separate-type-imports" },
      ],
      "no-restricted-globals": [
        "error",
        {
          name: "fetch",
          message: "Use the generated Connect clients via lib/transport.ts (AGENTS.md §7).",
        },
      ],
    },
  },
  {
    files: ["**/*.js"],
    extends: [tseslint.configs.disableTypeChecked],
    languageOptions: { globals: { ...globals.node } },
  },
  prettier,
);
