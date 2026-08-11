import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';

const sourceFiles = ['**/*.{ts,tsx}'];

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**', 'playwright-report/**', 'test-results/**'] },
  ...tseslint.configs.recommended.map((config) => ({ ...config, files: sourceFiles })),
  {
    files: sourceFiles,
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
  { files: ['**/*.{js,mjs,cjs}'], rules: { 'no-console': 'warn' } },
);
