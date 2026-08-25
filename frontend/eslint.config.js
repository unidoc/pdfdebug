// ESLint 9 flat config for the Wails React/TypeScript frontend.
// Strict TS + React + React Hooks rules, typed linting
// via typescript-eslint projectService, with a dedicated ignores entry.
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import pluginReact from 'eslint-plugin-react';
import pluginReactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';

export default [
  {
    ignores: [
      'dist/**',
      'bindings/**',
      'wailsjs/**',
      'node_modules/**',
      'eslint.config.js',
      'vite.config.*',
      'vitest.config.*',
      'public/**',
      '**/*.test.ts',
      '**/*.test.tsx',
      'src/test-setup.ts',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    files: ['**/*.{js,jsx,ts,tsx}'],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
        ecmaFeatures: { jsx: true },
      },
    },
    settings: {
      react: { version: 'detect' },
    },
    plugins: {
      react: pluginReact,
      'react-hooks': pluginReactHooks,
    },
    rules: {
      ...pluginReact.configs.flat.recommended.rules,
      ...pluginReactHooks.configs['recommended-latest'].rules,
      'no-console': 'warn',
      // Absolute rules that MUST NOT be relaxed.
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        caughtErrorsIgnorePattern: '^_',
      }],
      'react/react-in-jsx-scope': 'off',
      'react/prop-types': 'off',
      // Relaxed: untyped .jsx and untyped Wails binding surfaces trigger these.
      // Tracked for a follow-up story once bindings gain TS types.
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-floating-promises': 'off',
      '@typescript-eslint/no-misused-promises': 'off',
    },
  },
];
