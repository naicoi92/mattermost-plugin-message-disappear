// Babel config shared by webpack (browser bundle) and jest (node tests).
module.exports = {
    presets: [
        '@babel/preset-env',
        ['@babel/preset-react', {runtime: 'automatic'}],
        '@babel/preset-typescript',
    ],
};
