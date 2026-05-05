module.exports = {
  devServer: {
    port: 8080,
    compress: false,
    proxy: {
      '/api': {
        target: 'http://localhost:9090',
        changeOrigin: true,
        timeout: 0,
        proxyTimeout: 0,
        pathRewrite: {
          '^/api': '/api/v1'
        }
      }
    }
  }
}
