FROM ubuntu:14.04

RUN locale-gen en_US.UTF-8
RUN dpkg-reconfigure locales
ENV LANG en_US.UTF-8
ENV LC_ALL en_US.UTF-8

RUN apt-get update
RUN apt-get -y upgrade; apt-get clean

RUN apt-get install -y build-essential zlib1g-dev libssl-dev libxml2-dev libsqlite3-dev libxslt1-dev libpq-dev libmysqlclient-dev
RUN apt-get install -y git curl wget tar ruby ruby-dev
RUN apt-get clean

# package manager provides 1.4.3, which is too old for vagrant-aws
RUN cd /tmp && wget -q https://dl.bintray.com/mitchellh/vagrant/vagrant_1.6.5_x86_64.deb && dpkg -i vagrant_1.6.5_x86_64.deb
RUN vagrant plugin install vagrant-aws

# bosh-init dependencies
RUN apt-get install -y mercurial; apt-get clean
# ...go
ADD install-go.sh /tmp/install-go.sh
RUN chmod a+x /tmp/install-go.sh
RUN cd tmp; ./install-go.sh; rm install-go.sh
# ...ruby
ADD install-ruby.sh /tmp/install-ruby.sh
RUN chmod a+x /tmp/install-ruby.sh
RUN cd tmp; ./install-ruby.sh; rm install-ruby.sh

# lifecycle ssh test
RUN apt-get install -y sshpass; apt-get clean

# integration registry tests
RUN apt-get install -y openssh-server

# for hijack debugging
RUN apt-get install -y lsof psmisc strace
