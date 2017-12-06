/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import "github.com/spf13/pflag"

//NavarkoOptions holds data required for Navarkos config setup
type NavarkoOptions struct {
	FkubeApiServer string
	FkubeName      string
	FkubeNamespace string
}

//NewNO initializes NavarkoOptions with default values
func NewNO() *NavarkoOptions {
	no := NavarkoOptions{
		FkubeApiServer: "",
		FkubeName:      "federation",
		FkubeNamespace: "federation-system",
	}
	return &no
}

//AddFlags udpates NavarkoOptions with provided values
func (no *NavarkoOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&no.FkubeApiServer, "fkubeApiServer", no.FkubeApiServer, "Federation API host")
	fs.StringVar(&no.FkubeName, "fkubeName", no.FkubeName, "Federation Name")
	fs.StringVar(&no.FkubeNamespace, "fkubeNamespace", no.FkubeNamespace, "Federation Namespace")
}
