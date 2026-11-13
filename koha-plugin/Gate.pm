package Koha::Plugin::Com::Library::RfidGateApi::REST::V1::Gate;

use Modern::Perl;
use Mojo::Base 'Mojolicious::Controller';

use Mojo::JSON;
use POSIX     qw( ceil strftime );
use Try::Tiny qw( catch try );

use C4::Context;
use Koha::Plugin::Com::Library::RfidGateApi;

# ------------------------------------------------------------------
# GET /health
# ------------------------------------------------------------------
sub health {
    my $c = shift->openapi->valid_input or return;

    return try {
        C4::Context->dbh->do('SELECT 1');
        return $c->render( status => 200, openapi => { status => 'ok' } );
    }
    catch {
        warn "[RfidGateApi] health check failed: $_";
        return $c->render( status => 503, openapi => { status => 'error' } );
    };
}

# ------------------------------------------------------------------
# POST /rfid/check
# ------------------------------------------------------------------
sub check_item {
    my $c = shift->openapi->valid_input or return;

    my $plugin = Koha::Plugin::Com::Library::RfidGateApi->new;

    unless ( $c->_rga_authenticated($plugin) ) {
        return $c->render(
            status  => 401,
            openapi => { status => 'UNAUTHORIZED', message => 'Invalid or missing API key' }
        );
    }

    my $body = $c->req->json;
    return $c->render(
        status  => 400,
        openapi => { status => 'INVALID_REQUEST', message => 'Invalid JSON body' }
    ) unless ref $body eq 'HASH';

    my $rfid_tag = $body->{rfid_tag} // '';
    $rfid_tag =~ s/^\s+|\s+$//g;
    return $c->render(
        status  => 400,
        openapi => { status => 'INVALID_REQUEST', message => 'rfid_tag is required' }
    ) unless length $rfid_tag;

    return try {
        my $dbh = C4::Context->dbh;

        # Resolve tag -> item. SN prefix stripped on BOTH sides, same rule
        # as the legacy gate (replace(label,'SN','')), since the tag may or
        # may not carry it on either side of the comparison.
        #
        # Fast path: item_rfid.rfid_norm is a STORED generated column
        # (REPLACE(rfid,'SN','')) with an index, so this is an index lookup
        # instead of a scan. If the column is missing (e.g. the table was
        # recreated by another plugin), fall back to the original REPLACE()
        # query so the gate keeps working.
        ( my $norm = $rfid_tag ) =~ s/SN//g;

        my ( $itemnumber, $barcode );
        if ( _rga_has_norm_col($dbh) ) {
            ( $itemnumber, $barcode ) = $dbh->selectrow_array(
                q{
                    SELECT i.itemnumber, i.barcode
                    FROM item_rfid r
                    JOIN items i ON i.itemnumber = r.itemnumber
                    WHERE r.rfid_norm = ?
                    LIMIT 1
                },
                undef, $norm
            );
        }
        else {
            ( $itemnumber, $barcode ) = $dbh->selectrow_array(
                q{
                    SELECT i.itemnumber, i.barcode
                    FROM item_rfid r
                    JOIN items i ON i.itemnumber = r.itemnumber
                    WHERE REPLACE(r.rfid, 'SN', '') = REPLACE(?, 'SN', '')
                    LIMIT 1
                },
                undef, $rfid_tag
            );
        }

        # Fallback: direct barcode match (mirrors SLiMS's item_code fallback).
        unless ($itemnumber) {
            ( $itemnumber, $barcode ) = $dbh->selectrow_array(
                'SELECT itemnumber, barcode FROM items WHERE barcode = ? LIMIT 1',
                undef, $rfid_tag
            );
        }

        unless ($itemnumber) {
            # Unknown tag (wallet, clothes...): not a library item, not logged.
            $c->_rga_warn_masked( $rfid_tag, 'ITEM_NOT_FOUND' );
            return $c->render( status => 200, openapi => { status => 'ITEM_NOT_FOUND', rfid_tag => $rfid_tag } );
        }

        # Same "currently on loan" definition as Koha circulation: a row in issues.
        my ($issue_id) = $dbh->selectrow_array(
            'SELECT issue_id FROM issues WHERE itemnumber = ? LIMIT 1',
            undef, $itemnumber
        );

        my $status = $issue_id ? 'BORROWED' : 'NOT_BORROWED';
        $c->_rga_log_scan( $rfid_tag, $barcode, $status eq 'BORROWED' ? 0 : 1 );
        $c->_rga_warn_masked( $rfid_tag, $status );

        return $c->render( status => 200, openapi => { status => $status, rfid_tag => $rfid_tag } );
    }
    catch {
        warn "[RfidGateApi] check_item error: $_";
        return $c->render(
            status  => 500,
            openapi => { status => 'INTERNAL_ERROR', message => 'Unable to check item' }
        );
    };
}

# ------------------------------------------------------------------
# GET /gate/history
# ------------------------------------------------------------------
sub get_history {
    my $c = shift->openapi->valid_input or return;

    my $plugin = Koha::Plugin::Com::Library::RfidGateApi->new;

    unless ( $c->_rga_authenticated($plugin) ) {
        return $c->render(
            status  => 401,
            openapi => { error => Mojo::JSON->true, message => 'Invalid or missing API key' }
        );
    }

    my $page  = $c->param('page')  // 1;
    my $limit = $c->param('limit') // 20;
    $page  = 1  unless $page  =~ /^\d+$/ && $page >= 1;
    $limit = 20 unless $limit =~ /^\d+$/;
    $limit = 1   if $limit < 1;
    $limit = 200 if $limit > 200;
    my $offset = ( $page - 1 ) * $limit;

    my $item_code  = $c->param('itemCode') // '';
    my $title_kw   = $c->param('title')    // '';
    my $status_raw = $c->param('status');

    my $today      = strftime( '%Y-%m-%d', localtime );
    my $start_date = $c->param('startDate') // '2000-01-01';
    my $until_date = $c->param('untilDate') // $today;
    $start_date = '2000-01-01' unless $start_date =~ /^\d{4}-\d{2}-\d{2}$/;
    $until_date = $today       unless $until_date =~ /^\d{4}-\d{2}-\d{2}$/;

    my @conditions = ('DATE(l.scanned_at) BETWEEN ? AND ?');
    my @bind       = ( $start_date, $until_date );

    if ( length $item_code ) {
        push @conditions, q{(l.item_code = ? OR REPLACE(l.label_code,'SN','') = REPLACE(?,'SN',''))};
        push @bind, $item_code, $item_code;
    }

    if ( length $title_kw ) {
        for my $word ( split /\s+/, $title_kw ) {
            next unless length $word;
            push @conditions, 'bib.title LIKE ?';
            push @bind, "%$word%";
        }
    }

    if ( defined $status_raw && length $status_raw ) {
        push @conditions, 'l.status = ?';
        push @bind, $status_raw + 0;
    }

    my $where = 'WHERE ' . join( ' AND ', @conditions );
    my $dbh   = C4::Context->dbh;

    return try {
        my ($total) = $dbh->selectrow_array(
            qq{
                SELECT COUNT(*)
                FROM plugin_rfid_gate_log l
                LEFT JOIN items  i   ON i.barcode = l.item_code
                LEFT JOIN biblio bib ON bib.biblionumber = i.biblionumber
                $where
            },
            undef, @bind
        );

        my $rows = $dbh->selectall_arrayref(
            qq{
                SELECT l.scanned_at, l.item_code, l.label_code, bib.title, l.status
                FROM plugin_rfid_gate_log l
                LEFT JOIN items  i   ON i.barcode = l.item_code
                LEFT JOIN biblio bib ON bib.biblionumber = i.biblionumber
                $where
                ORDER BY l.scanned_at DESC
                LIMIT ? OFFSET ?
            },
            { Slice => {} }, @bind, $limit, $offset
        );

        my @records = map {
            {
                date         => $_->{scanned_at},
                item_code    => $_->{item_code},
                label_code   => $_->{label_code},
                title        => $_->{title},
                status       => $_->{status} + 0,
                status_label => $_->{status} == 0 ? 'Permitted' : 'Not Permitted',
            }
        } @{$rows};

        my $total_pages = $limit > 0 ? ceil( $total / $limit ) : 1;

        return $c->render(
            status  => 200,
            openapi => {
                error   => Mojo::JSON->false,
                message => 'Gate history retrieved',
                data    => {
                    total       => $total + 0,
                    page        => $page + 0,
                    limit       => $limit + 0,
                    total_pages => $total_pages + 0,
                    records     => \@records,
                },
            },
        );
    }
    catch {
        warn "[RfidGateApi] get_history error: $_";
        return $c->render(
            status  => 500,
            openapi => { error => Mojo::JSON->true, message => 'An internal server error occurred' }
        );
    };
}

# ------------------------------------------------------------------
# GET /rfid/status
#
# Feeds the gate middleware's offline mirror. Not logged to
# plugin_rfid_gate_log (it is a sync read, not a gate scan).
#
#  - no `since`  : FULL listing of every RFID-tagged item (also used by the
#                  client to reconcile deletions the delta cannot see).
#  - with `since`: only items whose items.timestamp (indexed; moves on
#                  checkout, checkin, renew, edit) or item_rfid.assigned_at
#                  is newer. Deleted items/tags are NOT visible here, so the
#                  client must run a full listing now and then.
#
# Pagination is keyset on the normalised tag (`after`), so deep pages stay
# cheap. `server_time` is Koha's own clock, read BEFORE the data query: the
# client sends it back as the next `since` (minus its own overlap window), so
# client/server clock skew and time zones never matter.
# ------------------------------------------------------------------
sub rfid_status {
    my $c = shift->openapi->valid_input or return;

    my $plugin = Koha::Plugin::Com::Library::RfidGateApi->new;

    unless ( $c->_rga_authenticated($plugin) ) {
        return $c->render(
            status  => 401,
            openapi => { status => 'UNAUTHORIZED', message => 'Invalid or missing API key' }
        );
    }

    my $since = $c->param('since') // '';
    my $after = $c->param('after') // '';
    my $limit = $c->param('limit') // 2000;
    $limit = 2000 unless $limit =~ /^\d+$/;
    $limit = 1    if $limit < 1;
    $limit = 5000 if $limit > 5000;

    if ( length $since ) {
        return $c->render(
            status  => 400,
            openapi => { status => 'INVALID_REQUEST', message => 'since must be YYYY-MM-DD HH:MM:SS' }
        ) unless $since =~ /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}$/;
        $since =~ s/T/ /;
    }

    return try {
        my $dbh = C4::Context->dbh;

        my ($server_time) = $dbh->selectrow_array(q{SELECT DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')});
        my ($total_rfid)  = $dbh->selectrow_array(q{SELECT COUNT(*) FROM item_rfid});

        my $norm     = _rga_has_norm_col($dbh) ? 'r.rfid_norm' : q{REPLACE(r.rfid, 'SN', '')};
        my $borrowed = q{EXISTS (SELECT 1 FROM issues s WHERE s.itemnumber = i.itemnumber)};
        my $fetch    = $limit + 1;    # one extra row tells us whether another page exists

        my $rows;
        if ( length $since ) {
            $rows = $dbh->selectall_arrayref(
                qq{
                    SELECT t.n, t.barcode, t.borrowed
                    FROM (
                        SELECT $norm AS n, i.barcode AS barcode, $borrowed AS borrowed
                        FROM items i JOIN item_rfid r ON r.itemnumber = i.itemnumber
                        WHERE i.timestamp > ?
                        UNION
                        SELECT $norm AS n, i.barcode AS barcode, $borrowed AS borrowed
                        FROM item_rfid r JOIN items i ON i.itemnumber = r.itemnumber
                        WHERE r.assigned_at > ?
                    ) t
                    WHERE t.n > ?
                    ORDER BY t.n
                    LIMIT ?
                },
                { Slice => {} }, $since, $since, $after, $fetch
            );
        }
        else {
            $rows = $dbh->selectall_arrayref(
                qq{
                    SELECT $norm AS n, i.barcode AS barcode, $borrowed AS borrowed
                    FROM item_rfid r JOIN items i ON i.itemnumber = r.itemnumber
                    WHERE $norm > ?
                    ORDER BY n
                    LIMIT ?
                },
                { Slice => {} }, $after, $fetch
            );
        }

        my $has_more = @{$rows} > $limit ? 1 : 0;
        splice @{$rows}, $limit if $has_more;

        my @records = map {
            {
                rfid_norm => $_->{n},
                item_code => $_->{barcode} // '',
                status    => $_->{borrowed} ? 'BORROWED' : 'NOT_BORROWED',
            }
        } @{$rows};

        return $c->render(
            status  => 200,
            openapi => {
                server_time => $server_time,
                since       => $since,
                total_rfid  => $total_rfid + 0,
                count       => scalar @records,
                has_more    => $has_more ? Mojo::JSON->true : Mojo::JSON->false,
                next_after  => $has_more ? $records[-1]{rfid_norm} : '',
                records     => \@records,
            },
        );
    }
    catch {
        warn "[RfidGateApi] rfid_status error: $_";
        return $c->render(
            status  => 500,
            openapi => { status => 'INTERNAL_ERROR', message => 'Unable to list item status' }
        );
    };
}

# ------------------------------------------------------------------
# GET /rfid/biblio
#
# Minimal bibliographic data for the gate dashboard, ONLY for RFID-tagged items
# that have a barcode: item barcode, biblionumber, a cleaned title, and whether
# a local cover image exists (cover_images). The cover itself is not sent; the
# dashboard builds its URL from the biblionumber (OPAC opac-image.pl), and only
# when has_cover is true because opac-image.pl answers a 1x1 gif otherwise.
#
# Same contract as /rfid/status: no `since` = FULL listing, with `since` = only
# what changed (items.timestamp, biblio.timestamp, cover_images.timestamp or
# item_rfid.assigned_at). Deletions are not visible to the delta, so clients run
# a full listing now and then. Keyset pagination on items.itemnumber (`after`).
# ------------------------------------------------------------------
sub rfid_biblio {
    my $c = shift->openapi->valid_input or return;

    my $plugin = Koha::Plugin::Com::Library::RfidGateApi->new;

    unless ( $c->_rga_authenticated($plugin) ) {
        return $c->render(
            status  => 401,
            openapi => { status => 'UNAUTHORIZED', message => 'Invalid or missing API key' }
        );
    }

    my $since = $c->param('since') // '';
    my $after = $c->param('after') // 0;
    my $limit = $c->param('limit') // 2000;
    $after = 0    unless $after =~ /^\d+$/;
    $limit = 2000 unless $limit =~ /^\d+$/;
    $limit = 1    if $limit < 1;
    $limit = 5000 if $limit > 5000;

    if ( length $since ) {
        return $c->render(
            status  => 400,
            openapi => { status => 'INVALID_REQUEST', message => 'since must be YYYY-MM-DD HH:MM:SS' }
        ) unless $since =~ /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}$/;
        $since =~ s/T/ /;
    }

    return try {
        my $dbh = C4::Context->dbh;

        my ($server_time) = $dbh->selectrow_array(q{SELECT DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')});
        my ($total) = $dbh->selectrow_array(
            q{SELECT COUNT(*) FROM item_rfid r JOIN items i ON i.itemnumber = r.itemnumber
              WHERE i.barcode IS NOT NULL AND i.barcode <> ''}
        );

        my $cols  = q{i.itemnumber AS id, i.barcode AS barcode, b.biblionumber AS biblionumber, b.title AS title,
                      EXISTS (SELECT 1 FROM cover_images c WHERE c.biblionumber = b.biblionumber) AS has_cover};
        my $base  = q{FROM item_rfid r
                      JOIN items i  ON i.itemnumber = r.itemnumber
                      JOIN biblio b ON b.biblionumber = i.biblionumber};
        my $valid = q{i.barcode IS NOT NULL AND i.barcode <> ''};
        my $fetch = $limit + 1;    # one extra row tells us whether another page exists

        my $rows;
        if ( length $since ) {
            $rows = $dbh->selectall_arrayref(
                qq{
                    SELECT t.id, t.barcode, t.biblionumber, t.title, t.has_cover
                    FROM (
                        SELECT $cols $base WHERE $valid AND i.timestamp > ?
                        UNION
                        SELECT $cols $base WHERE $valid AND b.timestamp > ?
                        UNION
                        SELECT $cols $base WHERE $valid AND r.assigned_at > ?
                        UNION
                        SELECT $cols $base WHERE $valid
                          AND EXISTS (SELECT 1 FROM cover_images c2
                                      WHERE c2.biblionumber = b.biblionumber AND c2.timestamp > ?)
                    ) t
                    WHERE t.id > ?
                    ORDER BY t.id
                    LIMIT ?
                },
                { Slice => {} }, $since, $since, $since, $since, $after, $fetch
            );
        }
        else {
            $rows = $dbh->selectall_arrayref(
                qq{
                    SELECT $cols $base
                    WHERE $valid AND i.itemnumber > ?
                    ORDER BY i.itemnumber
                    LIMIT ?
                },
                { Slice => {} }, $after, $fetch
            );
        }

        my $has_more = @{$rows} > $limit ? 1 : 0;
        splice @{$rows}, $limit if $has_more;
        my $next_after = $has_more ? $rows->[-1]{id} : '';

        my @records = map {
            ( my $title = $_->{title} // '' ) =~ s{\s*[/:;,=]+\s*$}{};    # drop trailing ISBD punctuation
            {
                item_code => $_->{barcode},
                biblio_id => "$_->{biblionumber}",
                title     => $title,
                has_cover => $_->{has_cover} ? Mojo::JSON->true : Mojo::JSON->false,
            }
        } @{$rows};

        return $c->render(
            status  => 200,
            openapi => {
                server_time => $server_time,
                since       => $since,
                total       => $total + 0,
                count       => scalar @records,
                has_more    => $has_more ? Mojo::JSON->true : Mojo::JSON->false,
                next_after  => "$next_after",
                records     => \@records,
            },
        );
    }
    catch {
        warn "[RfidGateApi] rfid_biblio error: $_";
        return $c->render(
            status  => 500,
            openapi => { status => 'INTERNAL_ERROR', message => 'Unable to list biblio data' }
        );
    };
}

# ------------------------------------------------------------------
# Internal helpers
# ------------------------------------------------------------------

# X-API-Key header, checked against the plugin's own configured key list.
# Fails closed if none configured — same contract as SLiMS's authenticate().
#
# NOTE: Authorization: Bearer <key> is intentionally NOT supported, unlike
# SLiMS. Verified directly (curl) against this Koha: Koha::REST::V1::Auth's
# authenticate_api_request() intercepts ANY request carrying an
# "Authorization: Bearer ..." header at the core level and attempts real
# OAuth2 client-credentials validation on it, throwing its own 401 before
# the request ever reaches this controller — regardless of whether the
# route requires auth. There is no way for a plugin route to see or
# override that header's value. X-API-Key is the only header that reaches
# this code.
sub _rga_authenticated {
    my ( $c, $plugin ) = @_;

    my $configured = $plugin->retrieve_data('rga_api_keys') // '';
    my @keys = grep { length } map { s/^\s+|\s+$//gr } split /\r?\n/, $configured;
    return 0 unless @keys;

    my $provided = $c->req->headers->header('X-API-Key');
    return 0 unless defined $provided && length $provided;

    return ( grep { $_ eq $provided } @keys ) ? 1 : 0;
}

# True when item_rfid.rfid_norm exists. Cached per worker process, but a
# negative answer is re-checked every 60s so adding the column needs no restart.
my ( $_rga_norm_ok, $_rga_norm_checked ) = ( 0, 0 );
sub _rga_has_norm_col {
    my ($dbh) = @_;
    return 1 if $_rga_norm_ok;
    return 0 if time - $_rga_norm_checked < 60;
    $_rga_norm_checked = time;
    my ($n) = eval {
        $dbh->selectrow_array(
            q{SELECT COUNT(*) FROM information_schema.COLUMNS
              WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'item_rfid' AND COLUMN_NAME = 'rfid_norm'}
        );
    };
    $_rga_norm_ok = ( $n // 0 ) ? 1 : 0;
    return $_rga_norm_ok;
}

# Best-effort insert — a logging failure must never turn a correct
# BORROWED/NOT_BORROWED/ITEM_NOT_FOUND answer into a 500.
sub _rga_log_scan {
    my ( $c, $raw_tag, $barcode, $status ) = @_;
    try {
        C4::Context->dbh->do(
            'INSERT INTO plugin_rfid_gate_log (label_code, item_code, status) VALUES (?, ?, ?)',
            undef, $raw_tag, $barcode, $status
        );
    }
    catch {
        warn "[RfidGateApi] failed to write gate log: $_";
    };
    return;
}

# Mask a scanned tag before it ever hits the Plack log: first/last 4 chars only.
sub _rga_warn_masked {
    my ( $c, $tag, $result ) = @_;
    my $len = length $tag;
    my $masked =
        $len <= 8
        ? ( '*' x $len )
        : substr( $tag, 0, 4 ) . ( '*' x ( $len - 8 ) ) . substr( $tag, -4 );
    warn "[RfidGateApi] tag=$masked result=$result";
    return;
}

1;
