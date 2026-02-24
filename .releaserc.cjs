module.exports = {
  branches: ['main'],
  tagFormat: 'v${version}',
  plugins: [
    '@semantic-release/commit-analyzer',
    '@semantic-release/release-notes-generator',
    [
      '@saithodev/semantic-release-gitea',
      {
        giteaUrl: process.env.GITEA_URL || 'https://github.com',
        giteaToken: process.env.GITEA_TOKEN,
      },
    ],
  ],
};
