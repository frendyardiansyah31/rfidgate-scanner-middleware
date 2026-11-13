package Koha::Plugin::Com::Library::RfidGateApi;

use Modern::Perl;
use base qw(Koha::Plugins::Base);

use C4::Context;
use JSON qw( decode_json );

our $VERSION = '1.0.6';

our $metadata = {
    name            => 'RFID Gate API',
    author          => 'UIII Library',
    description     => 'REST API for the RFID gate middleware: check whether an item is '
        . 'currently on loan by its RFID tag, a health probe, and gate scan history. '
        . 'rfid/check, rfid/status, rfid/biblio and gate/history require an X-API-Key; health does not.',
    date_authored   => '2026-10-02',
    date_updated    => '2026-11-11',
    minimum_version => '24.11',
    maximum_version => undef,
    version         => $VERSION,
};

sub new {
    my ( $class, $args ) = @_;
    $args->{'metadata'}            = $metadata;
    $args->{'metadata'}->{'class'} = $class;
    return $class->SUPER::new($args);
}

# ------------------------------------------------------------------
# install / upgrade / uninstall
# ------------------------------------------------------------------
sub install {
    my ( $self, $args ) = @_;
    my $dbh = C4::Context->dbh;
    $dbh->do(
        q{
        CREATE TABLE IF NOT EXISTS plugin_rfid_gate_log (
            id         INT(11)      NOT NULL AUTO_INCREMENT,
            scanned_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
            label_code VARCHAR(64)  NOT NULL,
            item_code  VARCHAR(20)  NULL,
            status     TINYINT(1)   NOT NULL,
            PRIMARY KEY (id),
            KEY idx_scanned_at (scanned_at),
            KEY idx_item_code (item_code)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
        }
    ) or die "Failed to create plugin_rfid_gate_log: " . $dbh->errstr;
    $self->_ensure_rfid_norm;
    return 1;
}

sub uninstall {
    my ( $self, $args ) = @_;
    C4::Context->dbh->do('DROP TABLE IF EXISTS plugin_rfid_gate_log');
    return 1;
}

sub upgrade {
    my ( $self, $args ) = @_;
    $self->_ensure_rfid_norm;
    return 1;
}

# Adds the indexed, generated item_rfid.rfid_norm column (tag without 'SN')
# used by Gate.pm for an index lookup. Never fatal: if the column cannot be
# created (e.g. item_rfid does not exist yet), Gate.pm falls back to the
# slower REPLACE() query, so the gate keeps working.
sub _ensure_rfid_norm {
    my ($self) = @_;
    my $dbh = C4::Context->dbh;
    eval {
        my ($table) = $dbh->selectrow_array(
            q{SELECT COUNT(*) FROM information_schema.TABLES
              WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'item_rfid'}
        );
        return 1 unless $table;
        my ($col) = $dbh->selectrow_array(
            q{SELECT COUNT(*) FROM information_schema.COLUMNS
              WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'item_rfid' AND COLUMN_NAME = 'rfid_norm'}
        );
        return 1 if $col;
        $dbh->do(
            q{ALTER TABLE item_rfid
              ADD COLUMN rfid_norm VARCHAR(64) AS (REPLACE(rfid,'SN','')) STORED,
              ADD INDEX idx_rfid_norm (rfid_norm)}
        );
        1;
    } or warn "[RfidGateApi] rfid_norm migration skipped: " . ( $@ || $dbh->errstr || 'unknown' );
    return 1;
}

# ------------------------------------------------------------------
# REST API — /api/v1/contrib/rfid_gate/...
# ------------------------------------------------------------------
sub api_namespace { return 'rfid_gate'; }

sub api_routes {
    my ($self) = @_;
    my $json = $self->mbf_read('openapi.json');
    return decode_json($json);
}

# ------------------------------------------------------------------
# configure — staff settings page: API key(s) for the gate middleware
# ------------------------------------------------------------------
sub configure {
    my ( $self, $args ) = @_;
    my $cgi = $self->{'cgi'};

    if ( $cgi->param('save') ) {
        my $keys = $cgi->param('rga_api_keys') // '';
        $self->store_data( { rga_api_keys => $keys } );
        print $cgi->redirect( $self->_plugin_url() . '&saved=1' );
        return;
    }

    my $template = $self->get_template( { file => 'configure.tt' } );
    $template->param(
        rga_api_keys => $self->retrieve_data('rga_api_keys') // '',
        saved        => $cgi->param('saved') // '',
    );
    $self->output_html( $template->output() );
}

sub _plugin_url {
    return '/cgi-bin/koha/plugins/run.pl'
        . '?class=Koha::Plugin::Com::Library::RfidGateApi'
        . '&method=configure';
}

1;
