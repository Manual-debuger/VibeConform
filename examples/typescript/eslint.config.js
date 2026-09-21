import js from '@eslint/js'
import eslintConfigPrettier from 'eslint-config-prettier'
import globals from 'globals'
import tseslint from 'typescript-eslint'

// Every .ts file is type-checked. A narrower glob is faster, but a file
// falling outside it is silently linted by nothing, and a lint run that
// checks less than you think it does still exits 0.
const typedFiles = ['**/*.ts']

// Config files are the exception, and they have to be. Tool configs
// (vitest.config.ts, drizzle.config.ts) normally sit outside every
// tsconfig's include, which under projectService makes them a fatal parse
// error rather than a lint finding. They get the untyped rules instead:
// still linted, just not type-aware.
const configFiles = ['**/*.config.ts', '**/*.config.*.ts']

export default tseslint.config(
  {
    ignores: ['**/dist/**', '**/coverage/**'],
  },
  {
    ...js.configs.recommended,
    files: ['**/*.{js,mjs}'],
    languageOptions: { globals: globals.node },
  },
  ...tseslint.configs.recommendedTypeChecked.map((config) => ({
    ...config,
    files: typedFiles,
    rules: {
      ...config.rules,
      '@typescript-eslint/require-await': 'off',
    },
    languageOptions: {
      ...config.languageOptions,
      globals: globals.node,
      parserOptions: {
        ...config.languageOptions?.parserOptions,
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  })),
  {
    ...tseslint.configs.disableTypeChecked,
    files: configFiles,
    languageOptions: {
      globals: globals.node,
      parserOptions: { projectService: false },
    },
  },
  eslintConfigPrettier,
)
