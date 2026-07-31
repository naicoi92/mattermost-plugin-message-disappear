module.exports = {
    testEnvironment: 'jsdom',
    transform: {
        '^.+\\.(js|jsx|ts|tsx)$': 'babel-jest',
    },
    moduleDirectories: ['src', 'node_modules'],
    moduleNameMapper: {
        '\\.(css|less|scss)$': '<rootDir>/tests/style_mock.js',
    },
    testPathIgnorePatterns: ['/node_modules/', '/dist/'],
    clearMocks: true,
};
