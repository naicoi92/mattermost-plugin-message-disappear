const path = require('path');

const PLUGIN_ID = require('../plugin.json').id;

// Externals are provided by the Mattermost webapp at runtime; bundling them
// would duplicate React/Redux. UI components that import these land in V2.3.
const config = {
    entry: ['./src/index.tsx'],
    resolve: {
        modules: ['src', 'node_modules'],
        extensions: ['*', '.js', '.jsx', '.ts', '.tsx'],
    },
    module: {
        rules: [
            {
                test: /\.(ts|tsx)$/,
                exclude: /node_modules/,
                use: {
                    loader: 'ts-loader',
                    options: {
                        transpileOnly: true,
                    },
                },
            },
        ],
    },
    externals: {
        react: 'React',
        'react-dom': 'ReactDOM',
        redux: 'Redux',
        'react-redux': 'ReactRedux',
        'prop-types': 'PropTypes',
        'react-router-dom': 'ReactRouterDom',
    },
    output: {
        devtoolNamespace: PLUGIN_ID,
        path: path.join(__dirname, 'dist'),
        publicPath: '/',
        filename: 'main.js',
    },
    mode: 'production',
};

module.exports = config;
