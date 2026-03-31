package datapagesgen

import "net/http"

// Mux returns the server's HTTP mux for registering custom routes.
func (s *Server) Mux() *http.ServeMux { return s.mux }

// SetAssetsDir overrides the static assets directory with an absolute path.
// This is needed when the process working directory may change at runtime
// (e.g. during codeparse), which breaks the relative dev-mode path.
func (s *Server) SetAssetsDir(dir string) {
	s.assetsFS = http.Dir(dir)
}
