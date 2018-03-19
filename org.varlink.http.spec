Name:           org.varlink.http
Version:        2
Release:        1%{?dist}
Summary:        Varlink HTTP Proxy
License:        ASL2.0
URL:            https://github.com/varlink/%{name}
Source0:        https://github.com/varlink/%{name}/archive/%{name}-%{version}.tar.gz
BuildRequires:  systemd
BuildRequires:  go
Requires:       com.redhat.resolver

%define debug_package %{nil}

%description
Varlink HTTP Proxy

%prep
%setup -T -c -n go/src/org.varlink.http
tar --strip-components=1 -x -f %{SOURCE0}

%build
mkdir -p build/src/github.com/varlink
ln -s $(pwd) build/src/github.com/varlink/%{name}
export GOPATH=$(pwd)/build
go build -ldflags "-X main.datadir=%{_datadir}/%{name}" github.com/varlink/%{name}

%install
install -d %{buildroot}%{_bindir}
install -d %{buildroot}%{_datadir}/%{name}
install -d %{buildroot}%{_unitdir}
install -m 0755 %{name} %{buildroot}%{_bindir}
install -m 0644 %{name}.service %{buildroot}%{_unitdir}
install -m 0644 %{name}.socket %{buildroot}%{_unitdir}
install -m 0644 static/* -t %{buildroot}%{_datadir}/%{name}

%post
%systemd_post %{name}.service %{name}.socket

%preun
%systemd_preun %{name}.service %{name}.socket

%postun
%systemd_postun

%files
%{_bindir}/%{name}
%{_unitdir}/%{name}.service
%{_unitdir}/%{name}.socket
%dir %{_datadir}/%{name}
%{_datadir}/%{name}/favicon.ico
%{_datadir}/%{name}/index.html
%{_datadir}/%{name}/interface.html
%{_datadir}/%{name}/method.html
%{_datadir}/%{name}/varlink.css

%changelog
* Mon Mar 19 2018 <info@varlink.org> 1-1
- org.varlink.http 2
