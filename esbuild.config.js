import * as esbuild from 'esbuild';
import {sassPlugin} from 'esbuild-sass-plugin';

const mode = process.argv[2] ?? 'all';
const enableWatcher = (process.argv[3] ?? null) === 'watch';
console.log('args:', process.argv, 'mode:', mode);

const exec = async (options) => {
    await esbuild.build(options).catch((reason) => console.error(reason) && process.exit(1));
    if (enableWatcher) {
        let ctx = await esbuild.context(options);
        await ctx.watch();
    }
};

const config = {
    entryPoints: ['./internal/ui/js/index.ts'],
    outfile: './internal/ui/static/index.mjs',
    bundle: true,
    minify: true,
    sourcemap: true,
    target: ['es2022'],
    logLevel: 'info',
    plugins: [],
    tsconfig: './tsconfig.json',
};

const cssConfig = {
    ...config,
    entryPoints: ['./internal/ui/styles/styles.scss'],
    outfile: './internal/ui/static/index.css',
    plugins: [sassPlugin({ quietDeps: true, embedded: true })],
    loader: {'.scss': 'css'},
    external: ['*.png', '*.jpg', '*.svg'],
};

if (['all', 'js'].includes(mode)) {
    await exec(config);
}

if (['all', 'css'].includes(mode)) {
    await exec(cssConfig);
}
