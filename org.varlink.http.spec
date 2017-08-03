%define build_date %(date +"%%a %%b %%d %%Y")
%define build_timestamp %(date +"%%Y%%m%%d.%%H%m%%S")

Name:           org.varlink.http
Version:        1
Release:        %{build_timestamp}%{?dist}
Summary:        Varlink HTTP Proxy
License:        ASL2.0
URL:            https://github.com/varlink/org.varlink.http
Source0:        https://github.com/varlink/org.varlink.http/archive/v%{version}.tar.gz
BuildRequires:  systemd
BuildRequires:  go
Requires:       org.varlink.resolver

%define debug_package %{nil}

%description
Varlink HTTP Proxy

%prep
%setup -T -c -n go/src/org.varlink.http
tar --strip-components=1 -x -f %{SOURCE0}

%build
go build -ldflags "-X main.datadir=%{_datadir}/%{name}" -o %{name}

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
* %{build_date} <info@varlink.org> %{version}-%{build_timestamp}
- %{name} %{version}
