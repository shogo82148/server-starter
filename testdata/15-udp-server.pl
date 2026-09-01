#!/usr/bin/env perl
# copied from https://github.com/kazuho/p5-Server-Starter/blob/81ab2b9a02c97952e184cf26aaddc404bfd7aea5/t/15-udp-server.pl

use strict;
use warnings;

use lib qw(blib/lib lib);
use IO::Socket::INET;
use Server::Starter qw(server_ports);

my $listener = IO::Socket::INET->new(
    Proto => 'udp',
);
$listener->fdopen((values %{server_ports()})[0], 'w')
    or die "failed to bind listening socket:$!";

$|= 1;
print "success\n";

sleep 100;
