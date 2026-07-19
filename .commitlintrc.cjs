module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    // Commit bodies and footers carry wrapped prose, URLs, and trailers that
    // routinely exceed the stock 100-char line limit; do not gate on them.
    'body-max-line-length': [0, 'always', Infinity],
    'footer-max-line-length': [0, 'always', Infinity],
  },
};
