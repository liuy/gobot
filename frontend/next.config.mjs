import { execSync } from 'child_process';

const getGitSha = () => {
  try {
    return execSync('git rev-parse --short HEAD').toString().trim();
  } catch {
    return 'dev';
  }
};

const isExport = process.env.NEXT_EXPORT === '1';
const nextDistDir = process.env.NEXT_DIST_DIR?.trim();

/** @type {import('next').NextConfig} */
const nextConfig = {
  devIndicators: false,
  images: {
    unoptimized: true,
  },
  env: {
    NEXT_PUBLIC_GIT_SHA: getGitSha(),
  },
  ...(nextDistDir && { distDir: nextDistDir }),
  ...(isExport && { output: 'export' }),

  // Dev mode: proxy /ws to backend
  async rewrites() {
    if (process.env.NODE_ENV === 'production') return [];
    return [
      {
        source: '/ws',
        destination: 'http://127.0.0.1:10086/ws',
      },
    ];
  },
}

export default nextConfig
