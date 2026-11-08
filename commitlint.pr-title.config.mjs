/**
 * commitlint configuration for pull request titles: the shared rules without commitlint's
 * default ignores. A title becomes the squash commit header on main, so headers such as
 * "Merge branch …", "Revert …" or a bare version must not pass unchecked. Local commits keep
 * the default ignores, as they are squashed away.
 *
 * @type {import('@commitlint/types').UserConfig}
 */
export default {
  extends: ['./commitlint.config.mjs'],
  defaultIgnores: false,
};
