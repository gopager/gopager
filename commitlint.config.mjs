/**
 * commitlint configuration: Conventional Commits as release-please reads them.
 *
 * Only what versioning and the changelog depend on is an error: a known lowercase type, an
 * optional scope, a non-empty description, "!" or a "BREAKING CHANGE:" footer for breaking
 * changes. Style rules of the conventional preset are warnings or switched off, so long URLs,
 * pasted logs, capitalized names and trailers never block a commit.
 *
 * Rule levels: 0 off, 1 warning, 2 error.
 *
 * @type {import('@commitlint/types').UserConfig}
 */
export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'header-max-length': [1, 'always', 100],
    'subject-case': [0],
    'subject-full-stop': [1, 'never', '.'],
    'scope-case': [0],
    'body-max-line-length': [0],
    'footer-max-line-length': [0],
  },
};
